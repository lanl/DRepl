#include <linux/slab.h>
#include <linux/uaccess.h>
#include <linux/sched.h>
#include <linux/wait.h>
#include "drepl.h"

#define CHUNKSZ	(64*1024*1024)

static void drepl_view_work(struct work_struct *work)
{
	int i, n;
	drepl_write_req *wr;
	drepl_block *b;

	wr = container_of(work, drepl_write_req, work);
//	if (wr->wq) {
//		printk(KERN_DEFAULT "drepl_view_work set_fs\n");
//		set_fs(wr->fs);
//	}

	for(i = wr->bidx; wr->datalen > 0 && i < wr->v->nblks; i++) {
		b = wr->v->blks[i];
		n = drepl_block_replicate(b, wr->data, wr->datalen, wr->offset, b->offset);
//		printk(KERN_DEFAULT "drepl_view_work offset %llu datalen %llu ret %d\n", wr->offset, wr->datalen, n);
		if (n < 0) {
			printk(KERN_ERR "drepl_view_work: error %d\n", n);
			break;
		}

		wr->offset += n;
		wr->data += n;
		wr->datalen -= n;
	}

	if (wr->rcnt)
		atomic_inc(wr->rcnt);

	if (wr->wq)
		wake_up(wr->wq);

//	set_fs(KERNEL_DS);
	vfree(wr);
}

s64 drepl_view_write(drepl_view *v, const char __user *data, u64 datalen, u64 offset)
{
	int i, bi, err, nwq, sync;
	s64 n, m, sz, len, dlen, o, chunksz;
	s64 ret;
	drepl_write_req *wr;
	atomic_t rcnt = ATOMIC_INIT(0);
	DECLARE_WAIT_QUEUE_HEAD(wq);
	atomic_t *prcnt;
	wait_queue_head_t *pwq;
	drepl_block *b;
	const char __user *d;

	prcnt = NULL;
	pwq = NULL;
	sync = 0;
	ret = 0;
	if (offset+datalen > v->size) {
		ret = -EFBIG;
		goto out;
	}

	/* save for writing to the original replica */
	d = data;
	dlen = datalen;
	o = offset;

	/* replication */
	sync = v->flags & VSYNC;

	/* even if async, we need one replica to be committed, so wait for all */
	if (!v->repl)
		sync = 1;

	if (sync) {
		prcnt = &rcnt;
		pwq = &wq;
	}

	bi = drepl_search_start(v, offset);
	nwq = 0;
	for(i = bi, n = datalen; n>0 && i<v->nblks; i++) {
		b = v->blks[i];
		if (b->ndest > 0)
			break;

		n -= b->size;
	}

	if (n<=0 || i>=v->nblks)
		goto write;

	while (datalen>0 && bi<v->nblks) {
		sz = 0;
		len = datalen;
		chunksz = len<CHUNKSZ?len:CHUNKSZ;
		for(i = bi; len>0 && sz < chunksz && i<v->nblks; i++) {
			b = v->blks[i];
			if (sz + b->size < chunksz) {
				sz += b->size;
				len -= b->size;
				continue;
			}

			if (b->elsize==0)
				break;

			m = b->elnum - (offset - b->offset) / b->elsize;
			n = (chunksz - sz) / b->elsize;
			if (n > m) {
				n = m;
				m = 0;
			} else {
				m -= n;
			}

			sz += n*b->elsize;
			len -= n*b->elsize;
			ret += n*b->elsize;
			if (m > 0)
				break;
		}

		if (sz==0) {
			if (len < b->elsize)
				break;

			printk(KERN_ERR "no progress i %d nblks %d len %lld elsize %lld\n", i, v->nblks, len, b->elsize);
			ret = -EFAULT;
			goto out;
		}

		wr = vmalloc(sizeof(*wr) + (sync?0:sz));
		if (!wr) {
			ret = -ENOMEM;
			goto out;
		}

		wr->v = v;
		wr->bidx = bi;
		wr->offset = offset;
		wr->datalen = sz;
		wr->wq = pwq;
		wr->rcnt = prcnt;
//		wr->fs = get_fs();
		INIT_WORK(&wr->work, drepl_view_work);
		if (pwq) {
			wr->data = data;
//			wr->fs = get_fs();
		} else {
			wr->data = (char *) &wr[1];
			if (copy_from_user((char *) wr->data, data, sz)) {
				vfree(wr);
				printk(KERN_ERR "can't copy_from_user sz %llu\n", sz);
				ret = -EFAULT;
				goto out;
			}
//			wr->fs = KERNEL_DS;
		}

//		printk(KERN_DEFAULT "drepl_view_write queue work offset %llu %llu pwq %p\n", offset, sz, pwq);
		queue_work(drepl_workqueue, &wr->work);
		offset += sz;
		data += sz;
		datalen -= sz;
		bi = i;
		nwq++;
	}

write:
	if (v->repl)
		ret = drepl_repl_write(v->repl, d, dlen, v->offset + o);

out:
	if (sync && nwq) {
		err = wait_event_interruptible(wq, (atomic_read(prcnt) >= nwq));
		if (err<0 && ret>=0) {
			ret = err;
			goto out;
		}
	}

//	printk(KERN_DEFAULT "drepl_view_write return %lld\n", ret);
	return ret;
}

s64 drepl_view_read(drepl_view *v, char __user *data, u64 dlen, u64 offset)
{
	int i, n, ret;
	drepl_block *b;
	s64 datalen;

	datalen = dlen;
	if (offset >= v->size) {
		return 0;
	}

	if (offset + datalen > v->size)
		datalen = v->size - offset;

	if (v->repl)
		return drepl_repl_read(v->repl, data, datalen, offset + v->offset);

	// unmaterialized view
	ret = 0;
	i = drepl_search_start(v, offset);
	while (i < v->nblks && datalen > 0) {
		b = v->blks[i];
//		printk(KERN_DEFAULT "drepl_view_read: block %d %p offset %llu datalen %llu\n", i, b, offset, datalen);
		if (offset > b->offset + b->size)
			break;

		n = drepl_block_read(b, data, datalen, offset, b->offset);
//		printk(KERN_DEFAULT "drepl_view_read: block read %d\n", n);
		if (n <= 0) {
			if (n==0) {
				printk(KERN_ERR "drepl_view_read: 0 read\n");
				n = -EIO;
			}

			return n;
		}

		ret += n;
		offset += n;
		data += n;
		datalen -= n;
		i++;
	}

//	printk(KERN_DEFAULT "drepl_view_read: return %d\n", ret);
	return ret;
}

// returns the index of the first block within the offset
int drepl_search_start(drepl_view *v, u64 offset)
{
	int n, b, t;
	u64 o;
	drepl_block **blks;

//	printk(KERN_DEFAULT "drepl_search_start offset %llu v %p nblks %d blks %p\n", offset, v, v->nblks, v->blks);
	blks = v->blks;
	n = 0;
	b = 0;
	t = v->nblks;
	while (b < t) {
		n = (b + t) / 2;
		o = blks[n]->offset;
//		printk(KERN_DEFAULT "drepl_search_start n %d o %d\n", n, 0);
		if (o == offset) {
			b = n + 1;
			break;
		} else if (o < offset) {
			b = n + 1;
		} else {
			t = b;
		}
	}

	if (b > 0) {
		b--;
	}

//	printk(KERN_DEFAULT "depl_search_start return %d\n", b);
	return b;
}
