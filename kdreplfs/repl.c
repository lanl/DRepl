#include <linux/fs.h>
#include <linux/file.h>
#include "drepl.h"

s64 drepl_repl_write(drepl_repl *r, const char __user *data, u64 datalen, u64 offset)
{
        loff_t ppos;

//	printk(KERN_DEFAULT "drepl_repl_write %d offset %llu datalen %llu\n", r->id, offset, datalen);
        ppos = offset;
        return vfs_write(r->file, data, datalen, &ppos);
}

s64 drepl_repl_read(drepl_repl *r, char __user *data, u64 datalen, u64 offset)
{
        loff_t ppos;

//	printk(KERN_DEFAULT "drepl_repl_read %d offset %llu datalen %llu\n", r->id, offset, datalen);
        ppos = offset;
        return vfs_read(r->file, data, datalen, &ppos);
}

int drepl_repl_init(drepl_repl *r)
{
        printk(KERN_DEFAULT "drepl_repl_init '%s''%s'\n", r->name, r->fname);
        r->file = filp_open(r->fname, O_RDWR | O_CREAT | O_LARGEFILE, 0660);
        r->f_path.mnt = r->file->f_path.mnt;
        r->f_path.dentry = r->file->f_path.dentry;
        if (IS_ERR(r->file)) {
                printk(KERN_DEFAULT "drepl_repl_init error %ld\n", PTR_ERR(r->file));
                return PTR_ERR(r->file);
        }

        printk(KERN_DEFAULT "drepl_repl_init success\n");
        return 0;
}

void drepl_repl_destroy(drepl_repl *r)
{
        if (r->file) {
                fput(r->file);
                r->file = NULL;
        }
}
