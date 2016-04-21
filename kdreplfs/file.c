/*
 * Copyright (c) 1998-2011 Erez Zadok
 * Copyright (c) 2009      Shrikar Archak
 * Copyright (c) 2003-2011 Stony Brook University
 * Copyright (c) 2003-2011 The Research oundation of SUNY
 * Copyright (c) 2012 Latchesar Ionkov <lionkov@lanl.gov>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 2 as
 * published by the Free Software Foundation.
 */

#include <linux/fs.h>
#include "drepl.h"
#include "dreplfs.h"
extern int force_sync(struct file* file, unsigned long vm_pgoff,
                      struct vm_area_struct *vma, unsigned long offset,
                      unsigned long end, unsigned long *pages);

static ssize_t dreplfs_read(struct file *file, char __user *buf,
                           size_t count, loff_t *ppos)
{
        int err;
        drepl_view *v;

        v = dreplfs_file_view(file);
//	printk(KERN_DEFAULT "dreplfs_read %p offset %llu count %lu\n", v, *ppos, count);
        err = drepl_view_read(v, buf, count, *ppos);
        if (err < 0) {
                return err;
        }

//	printk(KERN_DEFAULT "dreplfs_read return %d\n", err);
        *ppos += err;
        return err;
}

static ssize_t dreplfs_write(struct file *file, const char __user *buf,
                            size_t count, loff_t *ppos)
{
        int err;
        drepl_view *v;

//	printk(KERN_DEFAULT "dreplfs_write\n");
        v = dreplfs_file_view(file);
        err = drepl_view_write(v, buf, count, *ppos);
        if (err == 0) {
                // should write
                err = -EIO;
        }

        if (err < 0) {
                return err;
        }

        if (v->repl) {
                fsstack_copy_attr_times(file->f_path.dentry->d_inode, v->repl->file->f_path.dentry->d_inode);
        }

        *ppos += err;
        return err;
}

static int dreplfs_readdir(struct file *file, void *dirent, filldir_t filldir)
{
        int over, i;
        struct dreplfs_sb_info *sbd;
        drepl_view *v;

//	printk(KERN_DEFAULT "dreplfs_readdir\n");
        sbd = DREPLFS_SB(file->f_dentry->d_inode->i_sb);
        for(i = file->f_pos; i < sbd->d->nviews; i++) {
                v = &sbd->d->views[i];
                over = filldir(dirent, v->name, strlen(v->name), i, i + 1, DT_REG);
                if (over) {
                        return 0;
                }

                file->f_pos++;
        }

        return 0;
}

static int dreplfs_mmap(struct file *file, struct vm_area_struct *vma)
{
        int err = 0;
        bool willwrite;
        struct file *lower_file;
        const struct vm_operations_struct *saved_vm_ops = NULL;

        /* this might be deferred to mmap's writepage */
        willwrite = ((vma->vm_flags | VM_SHARED | VM_WRITE) == vma->vm_flags);

        /*
         * File systems which do not implement ->writepage may use
         * generic_file_readonly_mmap as their ->mmap op.  If you call
         * generic_file_readonly_mmap with VM_WRITE, you'd get an -EINVAL.
         * But we cannot call the lower ->mmap op, so we can't tell that
         * writeable mappings won't work.  Therefore, our only choice is to
         * check if the lower file system supports the ->writepage, and if
         * not, return EINVAL (the same error that
         * generic_file_readonly_mmap returns in that case).
         */
        lower_file = dreplfs_file_view(file)->repl[0].file;
        dreplfs_file_view(file)->repl->vma = vma;
        if (willwrite && !lower_file->f_mapping->a_ops->writepage) {
                err = -EINVAL;
                printk(KERN_ERR "dreplfs: lower file system does not "
                       "support writeable mmap\n");
                goto out;
        }

        /*
         * find and save lower vm_ops.
         *
         * XXX: the VFS should have a cleaner way of finding the lower vm_ops
         */

        vma->vm_file = get_file(lower_file);
        err = lower_file->f_op->mmap(lower_file, vma);
        file->f_dentry->d_sb->s_type->name="dreplfs";
        /* saved_vm_ops = vma->vm_ops; */
        /* vma->vm_ops = &dreplfs_vm_ops; */

        if (err) {
            printk(KERN_ERR "dreplfs: lower mmap failed %d\n", err);
            goto out;
        }

        /* if(dsb->mmap == NULL) */
        /*         dsb->mmap = &lower_file->f_mapping->i_mmap.rb_node; */
        /* else */
        /*         lower_file->f_mapping->i_mmap.rb_node = *(dsb->mmap); */

        /* err = do_munmap(current->mm, vma->vm_start, */
        /*                 vma->vm_end - vma->vm_start); */
        /* if (err) { */
        /*     printk(KERN_ERR "dreplfs: do_munmap failed %d\n", err); */
        /*     goto out; */
        /* } */

        /*
         * Next 3 lines are all I need from generic_file_mmap.  I definitely
         * don't want its test for ->readpage which returns -ENOEXEC.
         */

        file_accessed(file);

        if (!DREPLFS_F(file)->lower_vm_ops) /* save for our ->fault */
                DREPLFS_F(file)->lower_vm_ops = saved_vm_ops;

out:
        /* atomic_long_dec(&file->f_count); */
        return err;
}

static int dreplfs_open(struct inode *inode, struct file *file)
{
        struct dreplfs_file_info *dfi;

//	printk(KERN_DEFAULT "dreplfs_open\n");
        /* don't open unhashed/deleted files */
        if (d_unhashed(file->f_path.dentry)) {
                return -ENOENT;
        }

        dfi = kzalloc(sizeof(struct dreplfs_file_info), GFP_KERNEL);
        if (!dfi) {
                return -ENOMEM;
        }

        file->private_data = dfi;
        dfi->view = dreplfs_inode_view(file->f_dentry->d_inode);

//	printk(KERN_DEFAULT "dreplfs_open view %p\n", dfi->view);
        return 0;
}

static int dreplfs_open_dir(struct inode *inode, struct file *file)
{
        struct dreplfs_file_info *dfi;

//	printk(KERN_DEFAULT "dreplfs_open\n");
        /* don't open unhashed/deleted files */
        if (d_unhashed(file->f_path.dentry)) {
                return -ENOENT;
        }

        dfi = kzalloc(sizeof(struct dreplfs_file_info), GFP_KERNEL);
        if (!dfi) {
                return -ENOMEM;
        }

        file->private_data = dfi;
        dfi->view = dreplfs_inode_view(file->f_dentry->d_inode);
//	printk(KERN_DEFAULT "dreplfs_open view %p\n", dfi->view);
        return 0;
}

static int dreplfs_flush(struct file *file, fl_owner_t id)
{
/* TODO
        int err = 0;
        struct file *lower_file = NULL;

        lower_file = dreplfs_lower_file(file);
        if (lower_file && lower_file->f_op && lower_file->f_op->flush)
                err = lower_file->f_op->flush(lower_file, id);

        return err;
*/
        return 0;
}

/* release all lower object references & free the file info structure */
static int dreplfs_file_release(struct inode *inode, struct file *file)
{
//	printk(KERN_DEFAULT "dreplfs_release\n");
        kfree(DREPLFS_F(file));
        return 0;
}

static int dreplfs_fsync(struct file *file, loff_t start, loff_t end,
                        int datasync)
{
        /* struct file *lower_file = dreplfs_file_view(file)->repl[0].file; */
        struct vm_area_struct *vma = find_vma(current->mm, start);
        unsigned long pages;
        /* vma->vm_file = lower_file; */

        dump_stack();

        force_sync(file, vma->vm_pgoff, vma, start, end,
                   &pages);

        vma->vm_file = file;
        return 0;
}

static int dreplfs_aio_fsync(struct kiocb *file, int datasync)
{
        struct drepl_view *view = dreplfs_file_view(file->ki_filp);

        if (file->ki_pos+file->ki_nbytes > view->size) {
                if(view->size > file->ki_pos)
                        file->ki_nbytes = view->size-file->ki_pos;
                else
                        file->ki_nbytes = file->ki_pos - view->size;
        }

        file->ki_filp = view->repl->file;

        /* drepl_repl_write(view->repl, file->ki_buf, file->ki_nbytes, file->ki_pos); */
        /* drain_workqueue(drepl_workqueue); */

        return 0;
}

/*
static int dreplfs_fasync(int fd, struct file *file, int flag)
{
        int err = 0;
        struct file *lower_file = NULL;

        lower_file = dreplfs_lower_file(file);
        if (lower_file->f_op && lower_file->f_op->fasync)
                err = lower_file->f_op->fasync(fd, lower_file, flag);

        return err;
}
*/

const struct file_operations dreplfs_main_fops = {
        .llseek		= generic_file_llseek,
        .read		= dreplfs_read,
        .write		= dreplfs_write,
        .mmap		= dreplfs_mmap,
        .open		= dreplfs_open,
        .flush		= dreplfs_flush,
        .release	= dreplfs_file_release,
        .fsync		= dreplfs_fsync,
        /* .fasync		= dreplfs_fasync, */
        .aio_fsync = dreplfs_aio_fsync,
};

/* trimmed directory options */
const struct file_operations dreplfs_dir_fops = {
        .llseek		= generic_file_llseek,
        .read		= generic_read_dir,
        .readdir	= dreplfs_readdir,
        .open		= dreplfs_open_dir,
        .release	= dreplfs_file_release,
        /* .flush		= dreplfs_flush, */
        .fsync		= generic_file_fsync,
//	.fasync		= dreplfs_fasync,
};
