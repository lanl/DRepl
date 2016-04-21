/*
 * Copyright (c) 1998-2011 Erez Zadok
 * Copyright (c) 2009      Shrikar Archak
 * Copyright (c) 2003-2011 Stony Brook University
 * Copyright (c) 2003-2011 The Research Foundation of SUNY
 * Copyright (c) 2012 Latchesar Ionkov <lionkov@lanl.gov>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 2 as
 * published by the Free Software Foundation.
 */

#include <linux/module.h>
#include <linux/workqueue.h>
#include "drepl.h"
#include "dreplfs.h"

struct workqueue_struct *drepl_workqueue;

static int drepl_init(drepl *d)
{
        int i, err;

        err = 0;
        for(i = 0; i < d->nrepls; i++) {
                err = drepl_repl_init(&d->repls[i]);
                if (err)
                        break;
        }

        return err;
}

static void drepl_destroy(drepl *d)
{
        int i, j;
        drepl_repl *r;
        drepl_view *v;
        drepl_block *b;
        drepl_dest *dd;

        for(i = 0; i < d->nrepls; i++) {
                r = &d->repls[i];
                drepl_repl_destroy(r);
                kfree(r->name);
                kfree(r->fname);
        }

        for(i = 0; i < d->nviews; i++) {
                v = &d->views[i];
                kfree(v->name);
                kfree(v->blks);
        }

        for(i = 0; i < d->nblks; i++) {
                b = &d->blks[i];
                for(j = 0; j < b->ndest; j++) {
                        dd = &b->dest[j];
                        kfree(dd->expr);
                }

                kfree(b->dest);
                kfree(b->dim);
                kfree(b->fld);
        }

        kfree(d->repls);
        kfree(d->views);
        kfree(d->blks);
        kfree(d);
}

/*
 * There is no need to lock the dreplfs_super_info's rwsem as there is no
 * way anyone can have a reference to the superblock at this point in time.
 */
static int dreplfs_read_super(struct super_block *sb, void *raw_data, int silent)
{
        int err = 0;
        struct inode *inode;
        drepl *d;
        struct dreplfs_sb_info *dsb;

        d = drepl_import(raw_data);
        if (IS_ERR(d)) {
                err = PTR_ERR(d);
                goto out;
        }

//	printk(KERN_DEFAULT "drepfs: %d views %d replicas %d blocks\n", d->nviews, d->nrepls, d->nblks);

        err = drepl_init(d);
        if (err) {
                drepl_destroy(d);
                goto out;
        }

        /* allocate superblock private data */
        dsb = kzalloc(sizeof(struct dreplfs_sb_info), GFP_KERNEL);
        if (!dsb) {
                printk(KERN_CRIT "dreplfs: read_super: out of memory\n");
                err = -ENOMEM;
                goto out;
        }

        sb->s_fs_info = dsb;
        dsb->d = d;
        dsb->uid = current_fsuid();
        dsb->gid = current_fsgid();

        /* inherit maxbytes from lower file system */
        sb->s_maxbytes = MAX_LFS_FILESIZE;

        /*
         * Our c/m/atime granularity is 1 ns because we may stack on file
         * systems whose granularity is as good.
         */
        sb->s_time_gran = 1;
        sb->s_op = &dreplfs_sops;

        /* root inode */
        inode = new_inode(sb);
        if (!inode) {
                err = -ENOMEM;
                goto out_sput;
        }

        inode_init_owner(inode, NULL, S_IFDIR | 0777);
        inode->i_ino = 0;
        inode->i_blocks = 0;
        inode->i_rdev = 0;
        inode->i_atime = inode->i_mtime = inode->i_ctime = CURRENT_TIME;
        inode->i_mapping->a_ops = &dreplfs_aops;
        inode->i_op = &dreplfs_dir_iops;
        inode->i_fop = &dreplfs_dir_fops;

        /* root dentry */
        sb->s_root = d_make_root(inode);
        if (!sb->s_root) {
                err = -ENOMEM;
                goto out_iput;
        }
        d_set_d_op(sb->s_root, &dreplfs_dops);

        /* link the upper and lower dentries */
        sb->s_root->d_fsdata = NULL;
        d_rehash(sb->s_root);
        goto out; /* all is well */

        /* no longer needed: free_dentry_private_data(sb->s_root); */
        dput(sb->s_root);
out_iput:
        iput(inode);
out_sput:
        /* drop refs we took earlier */
        kfree(DREPLFS_SB(sb));
        sb->s_fs_info = NULL;

out:
//	printk(KERN_DEFAULT "dreplfs_read_super: exit %d\n", err);
        return err;
}

struct dentry *dreplfs_mount(struct file_system_type *fs_type, int flags,
                            const char *dev_name, void *raw_data)
{
        return mount_nodev(fs_type, flags, raw_data,
                           dreplfs_read_super);
}

static void dreplfs_kill_super(struct super_block *s)
{
        struct dreplfs_sb_info *dsb;

        printk(KERN_DEFAULT "dreplfs_kill_super: start flushing\n");
        flush_workqueue(drepl_workqueue);
        printk(KERN_DEFAULT "dreplfs_kill_super: end flushing\n");

        dsb = s->s_fs_info;
        s->s_fs_info = NULL;
        drepl_destroy(dsb->d);
        kfree(dsb);
        kill_anon_super(s);
}

static struct file_system_type dreplfs_fs_type = {
        .owner		= THIS_MODULE,
        .name		= DREPLFS_NAME,
        .mount		= dreplfs_mount,
        .kill_sb	= dreplfs_kill_super,
        .fs_flags	= FS_REVAL_DOT|FS_BINARY_MOUNTDATA,
};

static int __init init_dreplfs_fs(void)
{
        int err, ncpu, nactive;

        pr_info("Registering dreplfs " DREPLFS_VERSION "\n");
        err = dreplfs_init_inode_cache();
        if (err)
                return err;

        ncpu = num_possible_cpus();
        nactive = clamp_val(ncpu, 4, WQ_UNBOUND_MAX_ACTIVE);
        drepl_workqueue = alloc_workqueue("drepl", WQ_MEM_RECLAIM /* | WQ_UNBOUND*/, 2 /* nactive */);
//	drepl_workqueue = alloc_workqueue("drepl", WQ_MEM_RECLAIM,1);
        err = register_filesystem(&dreplfs_fs_type);
        if (err) {
                dreplfs_destroy_inode_cache();
                destroy_workqueue(drepl_workqueue);
        }
        return err;
}

static void __exit exit_dreplfs_fs(void)
{
        dreplfs_destroy_inode_cache();
        unregister_filesystem(&dreplfs_fs_type);
        destroy_workqueue(drepl_workqueue);
        pr_info("Completed dreplfs module unload\n");
}

MODULE_AUTHOR("Latchesar Ionkov <lionkov@lanl.gov>");
MODULE_DESCRIPTION("dreplfs " DREPLFS_VERSION);
MODULE_LICENSE("GPL");

module_init(init_dreplfs_fs);
module_exit(exit_dreplfs_fs);
