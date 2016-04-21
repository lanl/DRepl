/*
 * Copyright (c) 1998-2011 Erez Zadok
 * Copyright (c) 2009      Shrikar Archak
 * Copyright (c) 2003-2011 Stony Brook University
 * Copyright (c) 2003-2011 The Research Foundation of SUNY
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 2 as
 * published by the Free Software Foundation.
 */

#include <linux/fs.h>
#include "drepl.h"
#include "dreplfs.h"
#include <linux/mpage.h>
#include <linux/buffer_head.h>

static int dreplfs_fault(struct vm_area_struct *vma, struct vm_fault *vmf)
{
        int err;
        struct file *file, *lower_file;
        const struct vm_operations_struct *lower_vm_ops;
        struct vm_area_struct lower_vma;

        memcpy(&lower_vma, vma, sizeof(struct vm_area_struct));
        file = lower_vma.vm_file;
        lower_vm_ops = DREPLFS_F(file)->lower_vm_ops;
        BUG_ON(!lower_vm_ops);

        lower_file = dreplfs_file_view(file)->repl[0].file;
        /*
         * XXX: vm_ops->fault may be called in parallel.  Because we have to
         * resort to temporarily changing the vma->vm_file to point to the
         * lower file, a concurrent invocation of dreplfs_fault could see a
         * different value.  In this workaround, we keep a different copy of
         * the vma structure in our stack, so we never expose a different
         * value of the vma->vm_file called to us, even temporarily.  A
         * better fix would be to change the calling semantics of ->fault to
         * take an explicit file pointer.
         */
        lower_vma.vm_file = lower_file;
        err = lower_vm_ops->fault(&lower_vma, vmf);
        return err;
}

static int dreplfs_readpage(struct file *file, struct page *page)
{
    int ret =0;
    drepl_view *dv = dreplfs_file_view(file);
    struct file *lower_file = dv->repl[0].file;
    printk("dreplfs_readpage\n");

    dump_stack();
    if(dv->repl != 0) {
        if(lower_file->f_mapping->a_ops->readpage){
            /* page->mapping = lower_file->f_mapping; */
            ret= lower_file->f_mapping->a_ops->readpage(lower_file, page);
        }

        else
            panic("No READ PAGE func");
    }
    else
        panic("No Repl\n");

    /* page->mapping = file->f_mapping; */
    return ret;
}

static int dreplfs_writepage(struct page *page,
                          struct writeback_control *wbc) {
    int ret=0;
    struct inode *inode = page->mapping->host;
    struct drepl_view *v = dreplfs_inode_view(inode);
    struct file *lower_file = v->repl[0].file;

    dump_stack();

    page->mapping = lower_file->f_mapping;

    ret = page->mapping->a_ops->writepage(page, wbc);

    page->mapping = inode->i_mapping;

    return ret;
}

const struct address_space_operations dreplfs_aops = {
    .readpage = dreplfs_readpage,
    .writepage		= dreplfs_writepage,
};

const struct vm_operations_struct dreplfs_vm_ops = {
    .fault = dreplfs_fault,
};
