#include <linux/module.h>
#include <linux/slab.h>
#include <asm-generic/uaccess.h>

#include "drepl.h"

// char pbuf[8192];

static u8 *gint32(u8 *buf, u32 *v) {
	*v = buf[0] | (((u32)buf[1])<<8) | (((u32)buf[2])<<16) | (((u32)buf[3])<<24);
	return buf+4;
}

static u8 *gint64(u8 *buf, u64 *v) {
	*v = buf[0] | (((u64)buf[1])<<8) | (((u64)buf[2])<<16) | (((u64)buf[3])<<24) |
		(((u64)buf[4])<<32) | (((u64)buf[5])<<40) | (((u64)buf[6])<<48) | (((u64)buf[7])<<56);
	return buf+8;
}

static u8 *gstr(u8 *buf, char **v) {
	u32 len;
	char *s;

	buf = gint32(buf, &len);
	s = kzalloc(len + 1, GFP_KERNEL);
	memmove(s, buf, len);
	s[len] = '\0';
	*v = s;
//	printk(KERN_DEFAULT "gstr len %d string %s\n", len, s);
	return buf + len;
}

static u8 *gblk(u8 *buf, drepl_block **b, drepl *d) {
	u32 id;

	buf = gint32(buf, &id);
	if (id==0) {
		*b = NULL;
	} else {
		*b = &d->blks[id-1];
	}

	return buf;
}

static u8 *drepl_import_repl(drepl *d, u8 *buf) {
	u32 id;
	drepl_repl *r;

	buf = gint32(buf, &id);
//	printk(KERN_DEFAULT "dreplfs_import_repl id %d\n", id);

	r = &d->repls[id - 1];
	r->id = id - 1;
	buf = gstr(buf, &r->name);
	buf = gstr(buf, &r->fname);
//	printk(KERN_DEFAULT "dreplfs import replica id %d %p '%s'\n", id, r, r->name);
	return buf;
}

static u8 *drepl_import_view(drepl *d, u8 *buf) {
	u32 i, id;
	drepl_view *v;

	buf = gint32(buf, &id);
	v = &d->views[id - 1];
	v->id = id - 1;
	buf = gstr(buf, &v->name);
	buf = gint32(buf, &v->flags);
	buf = gint32(buf, &id);
	if (id == 0) {
		v->repl = NULL;
	} else {
		v->repl = &d->repls[id - 1];
	}

	buf = gint64(buf, &v->offset);
	buf = gint32(buf, &v->elo);
	buf = gint32(buf, &id);
	if (id == 0) {
		v->dflt = NULL;
	} else {
		v->dflt = &d->views[id - 1];
	}

	buf = gint32(buf, &v->nblks);
	v->blks = kzalloc(v->nblks * sizeof(drepl_block*), GFP_KERNEL);
	for(i = 0; i < v->nblks; i++) {
		buf = gblk(buf, &v->blks[i], d);
	}

//	printk(KERN_DEFAULT "dreplfs import view id %d %p '%s' nblks %d\n", id, v, v->name, v->nblks);
	return buf;
}

static u8 *drepl_import_dest(drepl *d, u8 *buf, drepl_dest *dd) {
	int i;
	drepl_expr *e;

//	printk(KERN_DEFAULT "dreplfs_import_dest %p\n", buf);
	buf = gint32(buf, &dd->nexpr);
	dd->expr = kzalloc(dd->nexpr*sizeof(drepl_expr), GFP_KERNEL);
	for(i = 0; i < dd->nexpr; i++) {
		e = &dd->expr[i];
		buf = gint64(buf, &e->a);
		buf = gint64(buf, &e->b);
		buf = gint64(buf, &e->c);
		buf = gint64(buf, &e->d);
		buf = gint32(buf, &e->xidx);
	}

	buf = gblk(buf, &dd->arr, d);
	buf = gblk(buf, &dd->el, d);

	return buf;
}

static u8 *drepl_import_block(drepl *d, u8 *buf) {
	int i, id;
	drepl_block *b;
//	u8 *bufstart;

//	bufstart = buf;
	buf = gint32(buf, &id);
	b = &d->blks[id - 1];
	b->id = id - 1;
	buf = gint32(buf, &id);
	if (id == 0) {
		b->view = NULL;
	} else {
		b->view = &d->views[id - 1];
	}

	buf = gint64(buf, &b->offset);
	buf = gint64(buf, &b->size);
	buf = drepl_import_dest(d, buf, &b->src);
	buf = gint32(buf, &b->ndest);
	printk(KERN_DEFAULT "dreplfs import block %d %p ndest %d\n", b->id, b, b->ndest);
	b->dest = kzalloc(b->ndest*sizeof(drepl_dest), GFP_KERNEL);
	for(i = 0; i < b->ndest; i++) {
		buf = drepl_import_dest(d, buf, &b->dest[i]);
	}

	buf = gint32(buf, &b->ndim);
	b->dim = kzalloc(b->ndim*sizeof(u64), GFP_KERNEL);
	for(i = 0; i < b->ndim; i++) {
		buf = gint64(buf, &b->dim[i]);
	}

	buf = gint64(buf, &b->elsize);
	buf = gint64(buf, &b->elnum);
	buf = gblk(buf, &b->el, d);

	buf = gint32(buf, &b->nfld);
	b->fld = kzalloc(b->nfld*sizeof(drepl_block*), GFP_KERNEL);
	for(i = 0; i < b->nfld; i++) {
		buf = gblk(buf, &b->fld[i], d);
	}

//	i = snprintf(pbuf, sizeof(pbuf), "block %d: ", b->id);
//	for(; bufstart != buf; bufstart++) {
//		i += snprintf(pbuf + i, sizeof(pbuf) - i, "%02x ", *bufstart);
//	}
//	printk(KERN_DEFAULT "%s\n", pbuf);

//	printk(KERN_DEFAULT "dreplfs import block %d %p ndest %d\n", b->id, b, b->ndest);
	return buf;
}

drepl *drepl_import(u8 *data) {
	int i, j;
	u32 buflen;
	u64 bufptr;
	u8 *buf;
	drepl *d;
	drepl_view *v;

	data = gint32(data, &buflen);
	data = gint64(data, &bufptr);

	buf = kmalloc(buflen, GFP_KERNEL);
	if (copy_from_user(buf, (const void __user *) bufptr, buflen)) {
		return ERR_PTR(-EFAULT);
	}

	printk(KERN_DEFAULT "drepl_import len %u ptr %llx buf %p\n", buflen, bufptr, buf);
//	for(i = 0; i < buflen; i++) {
//		printk(KERN_DEFAULT "%02x ", buf[i]);
//	}
//	printk(KERN_DEFAULT "\n");

	d = kzalloc(sizeof(*d), GFP_KERNEL);
	buf = gint32(buf, &d->nrepls);
	buf = gint32(buf, &d->nviews);
	buf = gint32(buf, &d->nblks);

	d->repls = kzalloc(d->nrepls * sizeof(drepl_repl), GFP_KERNEL);
	d->views = kzalloc(d->nviews * sizeof(drepl_view), GFP_KERNEL);
	d->blks = kzalloc(d->nblks * sizeof(drepl_block), GFP_KERNEL);

	for(i = 0; i < d->nblks; i++) {
		d->blks[i].id = i;
	}

	printk(KERN_DEFAULT "dreplfs import nrepl %d nview %d nblk %d\n", d->nrepls, d->nviews, d->nblks);
	for(i = 0; i < d->nrepls; i++) {
		buf = drepl_import_repl(d, buf);
	}

	for(i = 0; i < d->nviews; i++) {
		buf = drepl_import_view(d, buf);
	}

	for(i = 0; i < d->nblks; i++) {
		buf = drepl_import_block(d, buf);
	}

	/* calculate view sizes */
	for(i = 0; i < d->nviews; i++) {
		v = &d->views[i];
		for(j = 0; j < v->nblks; j++) {
			v->size += v->blks[j]->size;
		}
	}
/*
	printk(KERN_DEFAULT "Replicas:\n");
	for(i = 0; i < d->nrepls; i++) {
		drepl_repl *r;

		r = &d->repls[i];
		printk(KERN_DEFAULT "\t id %d '%s' '%s'\n", r->id, r->name, r->fname);
	}

	printk(KERN_DEFAULT "Views:\n");
	for(i = 0; i < d->nviews; i++) {
		v = &d->views[i];
		printk(KERN_DEFAULT "\tview id %d '%s' nblks %d\n", v->id, v->name, v->nblks);
		for(j = 0; j < v->nblks; j++) {
			drepl_block *b;

			b = v->blks[j];
			printk(KERN_DEFAULT "\t\tblock %d offset %llu\n", b->id, b->offset);
		}
	}
	
	printk(KERN_DEFAULT "Blocks:\n");
	for(i = 0; i < d->nblks; i++) {
		drepl_block *b;

		b = &d->blks[i];
		printk(KERN_DEFAULT "\tblock %d view %d ndest %d\n", b->id, b->view->id, b->ndest);

		if (b->el) {
			printk(KERN_DEFAULT "\t\telement id %d\n", b->el->id);
		}

		for(j = 0; j < b->nfld; j++) {
			printk(KERN_DEFAULT "\t\tfield id %d\n", b->fld[j]->id);
		}

		for(j = 0; j < b->ndest; j++) {
			int arrid, elid;

			arrid = -1;
			elid = -1;
			if (b->dest[j].arr) {
				arrid = b->dest[j].arr->id;
			}

			if (b->dest[j].el) {
				elid = b->dest[j].el->id;
			}

			printk(KERN_DEFAULT "\t\tdest arr %d el %d\n", arrid, elid);
		}
	}
*/
	return d;
}
