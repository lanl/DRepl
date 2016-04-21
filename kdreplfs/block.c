#include <linux/slab.h>
#include <asm/uaccess.h>
#include "drepl.h"

#define BUFSZ 1024*1024

static inline void drepl_block_xform(drepl_block *b, drepl_dest *d, u8 *src, u8 *dst, int nel);
static s64 drepl_ablock_read_seq(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base);
static s64 drepl_ablock_replicate_seq(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base);

static int drepl_is_seq(drepl_block *b, drepl_dest *d)
{
	int i;
	drepl_block *db;
	drepl_expr *e;

//	printk(KERN_DEFAULT "drepl_is_seq\n");
	db = d->arr;
	if (b->ndim != db->ndim) {
//		printk(KERN_DEFAULT "b->ndim %d db->ndim %d\n", b->ndim, db->ndim);
		return 0;
	}

	if (b->view->elo != db->view->elo) {
//		printk(KERN_DEFAULT "b elo %d db elo %d\n", b->view->elo, db->view->elo);
		return 0;
	}

	for(i = 0; i < d->nexpr; i++) {
		e = &d->expr[i];
		if (e->a*e->d != 1 || e->xidx != i) {
//			printk(KERN_DEFAULT "i %d e->a %lld e->b %lld e->c %lld e->d %lld e->xidx %d\n", i, e->a, e->b, e->c, e->d, e->xidx);
			return 0;
		}
	}

//	printk(KERN_DEFAULT "drepl_is_seq TRUE\n");
	return 1;
}

static inline int drepl_block_is_dest(drepl_block *b, drepl_block *d) {
	int i;

	for(i = 0; i < b->ndest; i++) {
		if (b->dest[i].arr == d) {
			return 1;
		}
	}

	return 0;
}

static s64 drepl_sblock_read(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base)
{
//	printk(KERN_DEFAULT "drepl_sblock_read %p offset %llu datalen %d\n", b, offset, datalen);
	if (b->view->repl) {
		return drepl_repl_read(b->view->repl, data, datalen, b->view->offset + offset);
	}

	// Unmaterialized view
	return drepl_block_read(b->src.arr, data, datalen, offset - base - b->offset + b->src.arr->offset, 0);
}

static s64 drepl_ablock_read(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base)
{
	u64 esz, eidx, sidx, soffset, doff;
	s64 idx1[16], didx1[16], *idx, *didx;
	s64 q, r;
	u8 buf1[64], *buf;
	drepl_dest *d;
	int ret, i, n, elo, buflen;
	mm_segment_t old_fs;

	if (b->view->repl) {
		return drepl_repl_read(b->view->repl, data, datalen, b->view->offset + offset);
	}

	// Unmaterialized view
//	printk(KERN_DEFAULT "drepl_ablock_read %d %p\n", b->id, b);
	if (b->src.arr == NULL)
		return -EIO;

	if (drepl_is_seq(b, &b->src))
		return drepl_ablock_read_seq(b, data, datalen, offset, base);

	offset -= base;
	esz = b->el->size;
	eidx = (offset + datalen) / esz;
	sidx = offset / esz;
	soffset = sidx * esz;
	datalen = esz - (offset - soffset) + (eidx - sidx - 1) * esz;

	if (ARRAY_SIZE(idx1) >= b->ndim) {
		idx = idx1;
	} else {
		idx = kmalloc(sizeof(s64)*b->ndim, GFP_KERNEL);
	}

	offset -= b->offset;
	d = &b->src;
	buflen = ARRAY_SIZE(buf1);
	buf = buf1;
	if (ARRAY_SIZE(idx1) >= d->nexpr) {
		didx = didx1;
	} else {
		didx = kmalloc(sizeof(s64)*d->nexpr, GFP_KERNEL);
	}

	ret = 0;
	elo = b->view->elo;
	old_fs = get_fs();
	set_fs(KERNEL_DS);
	while (datalen >= esz) {
//		printk(KERN_DEFAULT "drepl_ablock_read: sidx %llu\n", sidx);
		drepl_elo_toidx(elo, sidx, b->ndim, idx, b->dim);

		// calculate the indices in the destination array
		for(i = 0; i < d->nexpr; i++) {
			drepl_calc_expr(&d->expr[i], idx, &q, &r);
			if (r != 0) {
				// if there is a remainder, the element doesn't belong
				// to the destination array
				// TODO: error string???
				printk(KERN_ERR "boo\n");
				ret = -EFAULT;
				goto out;
			}

			didx[i] = q;
//			printk(KERN_DEFAULT "drepl_ablock_read didx: %d %lld\n", i, didx[i]);
		}

		doff = drepl_elo_fromidx(d->arr->view->elo, d->nexpr, didx, d->arr->dim) * d->arr->elsize;
//		printk(KERN_DEFAULT "drepl_ablock_read doff %llu\n", doff);
		if (buflen < d->arr->elsize) {
			if (buf != buf1) {
				kfree(buf);
			}

			buflen = d->arr->elsize;
			buf = kmalloc(buflen, GFP_KERNEL);
		}

		n = drepl_block_read(d->arr, buf, d->arr->elsize, d->arr->offset + doff, d->arr->offset + doff);
//		printk(KERN_DEFAULT "drepl_ablock_read block read %d elsize %llu\n", n, d->arr->elsize);
		if (n < 0) {
			ret = n;
			goto out;
		}

		if (n != d->arr->elsize) {
			// TODO: better error
			ret = -EIO;
			goto out;
		}

		drepl_block_xform(d->el, &d->el->dest[0], buf, data, 1);
		data += esz;
		datalen -= esz;
		offset += esz;
		sidx++;
		ret += esz;
//		printk(KERN_DEFAULT "drepl_ablock_read after xform datalen %d ret %d esz %llu\n", datalen, ret, esz);
	}

out:
	set_fs(old_fs);
	if (buf != buf1)
		kfree(buf);
	if (idx != idx1) {
		kfree(idx);
	}
	if (didx != didx1) {
		kfree(didx);
	}

//	printk(KERN_DEFAULT "drepl_ablock_read return %d\n", ret);
	return ret;
}

static s64 drepl_ablock_read_seq(drepl_block *b, u8 __user *data, u64 dlen, u64 offset, u64 base)
{
	u64 esz, eidx, sidx, soffset, doff;
	s64 idx1[16], didx1[16], *idx, *didx;
	s64 q, r, datalen, ret;
	u8 *buf;
	drepl_dest *d;
	int i, n, m, elo, buflen;
	mm_segment_t old_fs;

	datalen = dlen;
//	printk(KERN_ERR "drepl_ablock_read_seq data start %p end %p\n", data, data + dlen);
//	printk(KERN_DEFAULT "drepl_ablock_read_seq %p offset %llu datalen %lld\n", b, offset, datalen);
	old_fs = get_fs();
	set_fs(KERNEL_DS);
	offset -= base;
	esz = b->el->size;
	eidx = (offset + datalen) / esz;
	sidx = offset / esz;
	soffset = sidx * esz;
	datalen = esz - (offset - soffset) + (eidx - sidx - 1) * esz;

	if (ARRAY_SIZE(idx1) >= b->ndim) {
		idx = idx1;
	} else {
		idx = kmalloc(sizeof(s64)*b->ndim, GFP_KERNEL);
	}

	offset -= b->offset;
	d = &b->src;
	buflen = BUFSZ;
	buf = kmalloc(buflen, GFP_KERNEL);
	if (ARRAY_SIZE(idx1) >= d->nexpr) {
		didx = didx1;
	} else {
		didx = kmalloc(sizeof(s64)*d->nexpr, GFP_KERNEL);
	}

	ret = 0;
	elo = b->view->elo;
	drepl_elo_toidx(elo, sidx, b->ndim, idx, b->dim);

	// calculate the indices in the destination array
	for(i = 0; i < d->nexpr; i++) {
		drepl_calc_expr(&d->expr[i], idx, &q, &r);
		if (r != 0) {
			// if there is a remainder, the element doesn't belong
			// to the destination array
			// TODO: error string???
			printk(KERN_ERR "boo 2\n");
			ret = -EFAULT;
			goto out;
		}

		didx[i] = q;
	}

	doff = drepl_elo_fromidx(d->arr->view->elo, d->nexpr, didx, d->arr->dim) * d->arr->elsize;

//	printk(KERN_DEFAULT "drepl_ablock_read_seq doff %lld datalen %d esz %lld\n", doff, datalen, esz);
	while (datalen >= esz) {
		r = (datalen/esz) * d->arr->elsize;
		if (r > buflen)
			r = buflen;

//		printk(KERN_DEFAULT "drepl_ablock_read offset %llu len %lld datalen %lld esz %llu\n", d->arr->offset + doff, r, datalen, esz);
		n = drepl_block_read(d->arr, buf, r, d->arr->offset + doff, d->arr->offset + doff);
//		printk(KERN_DEFAULT "drepl_ablock_read block read %d\n", n);
		if (n < 0) {
			ret = n;
			printk(KERN_ERR "drepl_ablock_read err %d\n", n);
			goto out;
		} 

		if (n - (n%d->arr->elsize) == 0) {
			printk(KERN_DEFAULT "drepl_ablock_read short read offset %llu len %lld n %d elsize %llu\n", d->arr->offset + doff, datalen>buflen?buflen:datalen, n, d->arr->elsize);
			ret = -EIO;
			goto out;
		}

		n = n - (n%d->arr->elsize);	// should be elsize aligned, but just in case...
		m = n / d->arr->elsize;
//		printk(KERN_DEFAULT "drepl_ablock_read_seq: n %d m %d datalen %lld data %p\n", n, m, datalen, data);
		drepl_block_xform(d->el, &d->el->dest[0], buf, data, m);
		data += m*esz;
		datalen -= m*esz;
		offset += m*esz;
		sidx += m;
		ret += m*esz;
		doff += n;
//		printk(KERN_DEFAULT "drepl_ablock_read after xform datalen %d ret %d esz %llu\n", datalen, ret, esz);
	}

out:
	set_fs(old_fs);
	kfree(buf);
	if (idx != idx1) {
		kfree(idx);
	}
	if (didx != didx1) {
		kfree(didx);
	}

//	printk(KERN_DEFAULT "drepl_ablock_read_seq return %lld\n", ret);
	return ret;
}

static s64 drepl_tblock_read(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base)
{
	u8 sbuf1[64], dbuf1[64];
	u8 *sbuf, *dbuf;
	drepl_block *sb;
	s64 ret;
	mm_segment_t old_fs;

//	printk(KERN_DEFAULT "drepl_tblock_read %p offset %llu datalen %d\n", b, offset, datalen);
	if (b->view->repl) {
		return drepl_repl_read(b->view->repl, data, datalen, b->view->offset + offset);
	}

	// Unmaterialized view
	old_fs = get_fs();
	set_fs(KERNEL_DS);
	sb = b->src.arr;
	if (ARRAY_SIZE(sbuf1) <= sb->size) {
		sbuf = sbuf1;
	} else {
		sbuf = kmalloc(sb->size, GFP_KERNEL);
	}

	if (ARRAY_SIZE(dbuf1) <= b->size) {
		dbuf = dbuf1;
	} else {
		dbuf = kmalloc(b->size, GFP_KERNEL);
	}

	ret = drepl_block_read(sb, sbuf, sb->size, sb->offset, sb->offset);
	if (ret < 0) {
		goto out;
	}

	if (ret != sb->size) {
		// TODO: better error code
		ret = -EIO;
		goto out;
	}

	drepl_block_xform(b, &b->dest[0], sbuf, dbuf, 1);
	ret = b->size - offset + base;
	memmove(data, &dbuf[offset - base], ret);

out:
	set_fs(old_fs);
	if (sbuf != sbuf1) {
		kfree(sbuf);
	}

	if (dbuf != dbuf1) {
		kfree(dbuf);
	}

	return ret;
}

s64 drepl_block_read(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base)
{
	if (b->ndim > 0) {
		return drepl_ablock_read(b, data, datalen, offset, base);
	} else if (b->nfld > 0) {
		return drepl_tblock_read(b, data, datalen, offset, base);
	} else {
		return drepl_sblock_read(b, data, datalen, offset, base);
	}
}

static void drepl_sblock_xform(drepl_block *b, drepl_dest *d, u8 *src, u8 *dst, int nel)
{
//	printk(KERN_DEFAULT "drepl_sblock_xform %d dest arr %d el %d nel %d\n", b->id, d->arr?d->arr->id:-1, d->el?d->el->id:-1, nel);
	memmove(dst, src, b->size * nel);
}

static void drepl_ablock_xform(drepl_block *b, drepl_dest *d, u8 *src, u8 *dst, int nel)
{
	s64 idx1[16], didx1[16], *idx, *didx;
	s64 q, r;
	u64 soff, doff, n, dn;
	int i, elo;

//	printk(KERN_DEFAULT "drepl_ablock_xform %d dest arr %d el %d nel %d\n", b->id, d->arr?d->arr->id:-1, d->el?d->el->id:-1, nel);
	if (ARRAY_SIZE(idx1) >= b->ndim) {
		idx = idx1;
	} else {
		idx = kmalloc(sizeof(s64) * b->ndim, GFP_KERNEL);
	}

	if (ARRAY_SIZE(didx1) >= d->nexpr) {
		didx = didx1;
	} else {
		didx = kmalloc(sizeof(s64) * d->nexpr, GFP_KERNEL);
	}

	elo = b->view->elo;
	while (nel) {
		for(n = 0; n < b->elnum; n++) {
			drepl_elo_toidx(elo, n, b->ndim, idx, b->dim);
			for(i = 0; i < d->nexpr; i++) {
				drepl_calc_expr(&d->expr[i], idx, &q, &r);
				if (r != 0) {
					// if there is a remainder, the element doesn't belong to the destination array
					break;
				}

				didx[i] = q;
			}

			if (i < d->nexpr) {
				continue;
			}

			dn = drepl_elo_fromidx(d->arr->view->elo, d->nexpr, didx, d->arr->dim);
			soff = n * b->elsize;
			doff = dn * d->arr->elsize;
			drepl_block_xform(b->el, &b->el->dest[0], &src[soff], &dst[doff], 1);
		}

		src += d->arr->size;
		dst += b->size;
		nel--;
	}

	if (idx != idx1) {
		kfree(idx);
	}

	if (didx != didx1) {
		kfree(didx);
	}
}

static void drepl_tblock_xform(drepl_block *b, drepl_dest *d, u8 *src, u8 *dst, int nel)
{
	u64 n, m;
	drepl_block *bb, *db, *dbb;
	int i, j;

//	printk(KERN_DEFAULT "drepl_tblock_xform %d dest arr %d el %d nel %d src %02x%02x%02x%02x\n", b->id, d->arr?d->arr->id:-1, d->el?d->el->id:-1, nel, src[0], src[1], src[2], src[3]);
	db = d->arr->el;
	while (nel) {
		n = 0;
		for(i = 0; i < b->nfld; i++) {
			m = 0;
			bb = b->fld[i];
//			printk(KERN_DEFAULT "drepl_tblock_xform src fld %d id %d\n", i, bb->id);
			
			// ugly but will do for now
			for(j = 0; j < db->nfld; j++) {
				dbb = db->fld[j];
//				printk(KERN_DEFAULT "drepl_tblock_xform \t dst fld %d id %d\n", j, dbb->id);
				if (drepl_block_is_dest(bb, dbb)) {
					drepl_block_xform(bb, &bb->dest[0], &src[n], &dst[m], 1);
					break;
				}

				m += dbb->size;
			}

			n += bb->size;
		}

		src += b->size;
		dst += db->size;
		nel--;
	}
}

static inline void drepl_block_xform(drepl_block *b, drepl_dest *d, u8 *src, u8 *dst, int nel)
{
	if (b->ndim > 0) {
		drepl_ablock_xform(b, d, src, dst, nel);
	} else if (b->nfld > 0) {
		drepl_tblock_xform(b, d, src, dst, nel);
	} else {
		drepl_sblock_xform(b, d, src, dst, nel);
	}
}

static s64 drepl_sblock_replicate(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base)
{
	u64 dbase, doff;
	drepl_block *d;
	int i;
	s64 ret;

//	printk(KERN_DEFAULT "drepl_sblock_replicate %d\n", b->id);
	dbase = offset - base + b->offset;
	doff = offset - b->offset;
	for(i = 0; i < b->ndest; i++) {
		d = b->dest[i].arr;
		ret = drepl_block_write(d, data, datalen, doff + d->offset, dbase + d->offset, 1, 0);
		if (ret < 0) {
			return ret;
		}
	}

	return b->size;
}

static s64 drepl_ablock_replicate(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base)
{
	u64 esz, o, doff;
	s64 idx1[16], didx1[16], *idx, *didx, q, r, ret, n;
	int elo, nd, i, j;
	drepl_dest *d;

	for(i = 0; i < b->ndest; i++)
		if (!drepl_is_seq(b, &b->dest[i]))
			break;

	if (i >= b->ndest)
		return drepl_ablock_replicate_seq(b, data, datalen, offset, base);

//	printk(KERN_DEFAULT "drepl_ablock_replicate %d offset %llu base %llu datalen %llu\n", b->id, offset, base, datalen);
	esz = b->el->size;
	elo = b->view->elo;
	offset -= base;
	o = offset - (offset / b->elsize) * b->elsize;
	if (ARRAY_SIZE(idx1) >= b->ndim) {
		idx = idx1;
	} else {
		idx = kmalloc(sizeof(s64)*b->ndim, GFP_KERNEL);
	}

	nd = ARRAY_SIZE(didx1);
	didx = didx1;

	ret = 0;
	while (datalen >= esz) {
		drepl_elo_toidx(elo, offset / b->elsize, b->ndim, idx, b->dim);

		for(i=0; i < b->ndest; i++) {
			// calculate indices
			d = &b->dest[i];
			if (nd < d->nexpr) {
				if (didx != didx1) {
					kfree(didx);
					didx = NULL;
				}

				nd = d->nexpr;
				didx = kmalloc(sizeof(s64) * nd, GFP_KERNEL);
			}

			for(j=0; j<d->nexpr; j++) {
				drepl_calc_expr(&d->expr[j], idx, &q, &r);
				if (r != 0) {
					// if there is a remainder, the element doesn't belong
					// to the destination array
					break;
				}

				didx[j] = q;
			}


			if (j < d->nexpr) {
				continue;
			}

			doff = drepl_elo_fromidx(d->arr->view->elo, d->nexpr, didx, d->arr->dim) * d->arr->elsize;
			n = drepl_block_replicate(d->el, data, esz, d->arr->offset + doff + o, d->arr->offset + doff);
			if (n < 0) {
				printk("drepl_ablock_replicate: error %lld\n", n);
				ret = n;
				goto out;
			}
		}

		o = 0;
		data += b->elsize;
		datalen -= b->elsize;
		offset += b->elsize;
		ret += b->elsize;
	}

out:
	if (idx != idx1) {
		kfree(idx);
	}

	if (didx != didx1) {
		kfree(didx);
	}

//	printk(KERN_DEFAULT "drepl_ablock_replicate %d return %lld\n", b->id, ret);
	return ret;
}

static s64 drepl_ablock_replicate_seq(drepl_block *b, const char __user *dat, u64 dlen, u64 offset, u64 base)
{
	u64 esz, o, doff;
	s64 idx1[16], didx1[16], *idx, *didx, q, r, n, datalen, ret;
	int elo, nd, i, j, m, buflen;
	u8 *buf;
	const char __user *data;
	drepl_dest *d;
	drepl_view *v;

//	printk(KERN_DEFAULT "drepl_ablock_replicate_seq %d offset %llu base %llu datalen %llu\n", b->id, offset, base, dlen);
	ret = dlen;
	esz = b->el->size;
	elo = b->view->elo;
	offset -= base;
	o = offset - (offset / b->elsize) * b->elsize;
	if (ARRAY_SIZE(idx1) >= b->ndim) {
		idx = idx1;
	} else {
		idx = kmalloc(sizeof(s64)*b->ndim, GFP_KERNEL);
	}

	nd = ARRAY_SIZE(didx1);
	didx = didx1;
	buflen = BUFSZ;
	buf = kmalloc(buflen, GFP_KERNEL);

	for(i = 0; i < b->ndest; i++) {
		d = &b->dest[i];
		data = dat;
		datalen = dlen;
		drepl_elo_toidx(elo, o / b->elsize, b->ndim, idx, b->dim);

//		printk(KERN_DEFAULT "drepl_ablock_replicate_seq dest %d arr %d el %d\n", i, d->arr ? d->arr->id:-1, d->el ? d->el->id:-1);
		if (nd < d->nexpr) {
			if (didx != didx1) {
				kfree(didx);
				didx = NULL;
			}

			nd = d->nexpr;
			didx = kmalloc(sizeof(s64) * nd, GFP_KERNEL);
		}

		for(j=0; j<d->nexpr; j++) {
			drepl_calc_expr(&d->expr[j], idx, &q, &r);
			didx[j] = q;
		}

		doff = drepl_elo_fromidx(d->arr->view->elo, d->nexpr, didx, d->arr->dim) * d->arr->elsize;
		while (datalen >= esz) {
			n = datalen > buflen ? buflen : datalen;
			n = datalen - (datalen%esz);	// elsize aligned
			m = n / esz;
			if (m*d->arr->elsize > buflen) {
				m = buflen / d->arr->elsize;
				n = m * esz;
			}

			if (n == 0) {
				printk(KERN_DEFAULT "drepl_ablock_replicate_seq: n == 0 datalen %lld buflen %d elsize %lld\n", datalen, buflen, d->arr->elsize);
				ret = -EIO;
				goto out;
			}

			v = d->arr->view;
			if (esz < d->arr->elsize) {
				// if we are writing partial data, read first
				j = drepl_repl_read(v->repl, buf, m*d->arr->elsize, v->offset + doff);
				if (j != m*d->arr->elsize) {
					printk(KERN_DEFAULT "short write %d\n", j);
					if (j < 0)
						ret = j;
					else
						ret = -EIO;

					goto out;
				}
			}

			drepl_block_xform(d->el, d, (u8 *) data, buf, m);
			drepl_repl_write(v->repl, buf, m*d->arr->elsize, v->offset + d->arr->offset + doff);
			data += n;
			datalen -= n;
			doff += m*d->arr->elsize;
		}
	}

out:
	if (idx != idx1) {
		kfree(idx);
	}

	if (didx != didx1) {
		kfree(didx);
	}

	kfree(buf);

//	printk(KERN_DEFAULT "drepl_ablock_replicate return %lld\n", ret);
	return ret; // is this correct???
}

static s64 drepl_tblock_replicate(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base)
{
	int i;
	s64 n, ret;
	drepl_block *bb;

//	printk(KERN_DEFAULT "drepl_tblock_replicate %d\n", b->id);
	ret = 0;
	for(i = 0; i < b->nfld; i++) {
		bb = b->fld[i];
		n = drepl_block_replicate(bb, data, datalen, offset, base);
		if (n < 0) {
			return n;
		}

		n = b->size;
		offset += n;
		base += n;
		ret += n;
		data += n;
		datalen -= n;

		if (datalen <= 0) {
			break;
		}
	}

	return ret;
}

s64 drepl_block_replicate(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base)
{
	if (b->ndest == 0)
		return datalen;

	if (b->ndim > 0) {
		return drepl_ablock_replicate(b, data, datalen, offset, base);
	} else if (b->nfld > 0) {
		return drepl_tblock_replicate(b, data, datalen, offset, base);
	} else {
		return drepl_sblock_replicate(b, data, datalen, offset, base);
	}
}

s64 drepl_block_write(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base, int w, int r)
{
	int ret;

	printk(KERN_DEFAULT "drepl_block_write %d offset %llu base %llu datalen %llu\n", b->id, offset, base, datalen);
	if (w && b->view->repl != NULL) {
		ret = drepl_repl_write(b->view->repl, data, datalen, b->view->offset + offset);
		if (!r || ret < 0) {
			return ret;
		}
	} else if (!r) {
		return b->size;
	}

	ret = drepl_block_replicate(b, data, datalen, offset, base);
	return ret;
}

