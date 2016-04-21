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

#ifndef _DREPLFS_H_
#define _DREPLFS_H_

#include <linux/dcache.h>
#include <linux/file.h>
#include <linux/fs.h>
#include <linux/mm.h>
#include <linux/mount.h>
#include <linux/namei.h>
#include <linux/seq_file.h>
#include <linux/statfs.h>
#include <linux/fs_stack.h>
#include <linux/magic.h>
#include <linux/uaccess.h>
#include <linux/slab.h>
#include <linux/sched.h>

/* the file system name */
#define DREPLFS_NAME "dreplfs"

/* dreplfs root inode number */
#define DREPLFS_ROOT_INO     1

/* useful for tracking code reachability */
#define UDBG(s) printk(KERN_DEFAULT "DBG:%s:%s:%d\n", __FILE__, __func__, __LINE__, s)

/* operations vectors defined in specific files */
extern const struct file_operations dreplfs_main_fops;
extern const struct file_operations dreplfs_dir_fops;
extern const struct inode_operations dreplfs_main_iops;
extern const struct inode_operations dreplfs_dir_iops;
extern const struct inode_operations dreplfs_symlink_iops;
extern const struct super_operations dreplfs_sops;
extern const struct dentry_operations dreplfs_dops;
extern const struct address_space_operations dreplfs_aops, dreplfs_dummy_aops;
extern const struct vm_operations_struct dreplfs_vm_ops;

extern int dreplfs_init_inode_cache(void);
extern void dreplfs_destroy_inode_cache(void);
extern struct dentry *dreplfs_lookup(struct inode *dir, struct dentry *dentry, unsigned int flags);
struct inode *dreplfs_iget(struct super_block *sb, drepl_view *v);

/* file private data */
struct dreplfs_file_info {
        drepl_view	*view;
        const struct vm_operations_struct *lower_vm_ops;
};

/* dreplfs inode data in memory */
struct dreplfs_inode_info {
        drepl_view	*view;
        struct inode	vfs_inode;
};

/* dreplfs super-block data in memory */
struct dreplfs_sb_info {
        drepl			*d;
        unsigned int		uid;
        unsigned int		gid;
};

/*
 * inode to private data
 *
 * Since we use containers and the struct inode is _inside_ the
 * dreplfs_inode_info structure, DREPLFS_I will always (given a non-NULL
 * inode pointer), return a valid non-NULL pointer.
 */
static inline struct dreplfs_inode_info *DREPLFS_I(const struct inode *inode)
{
        return container_of(inode, struct dreplfs_inode_info, vfs_inode);
}

/* superblock to private data */
#define DREPLFS_SB(super) ((struct dreplfs_sb_info *)(super)->s_fs_info)

/* file to private Data */
#define DREPLFS_F(file) ((struct dreplfs_file_info *)((file)->private_data))

static inline drepl_view *dreplfs_file_view(const struct file *f) {
        return DREPLFS_F(f)->view;
}

static inline drepl_view *dreplfs_inode_view(const struct inode *i)
{
        return DREPLFS_I(i)->view;
}

static inline drepl *dreplfs_super_drepl(const struct super_block *sb)
{
        return DREPLFS_SB(sb)->d;
}

#endif	/* not _DREPLFS_H_ */
