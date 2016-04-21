/*
 * Copyright (c) 1998-2011 Erez Zadok
 * Copyright (c) 2009	   Shrikar Archak
 * Copyright (c) 2003-2011 Stony Brook University
 * Copyright (c) 2003-2011 The Research Foundation of SUNY
 * Copyright (c) 2012 Latchesar Ionkov <lionkov@lanl.gov>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 2 as
 * published by the Free Software Foundation.
 */

#include <linux/fs.h>
#include "drepl.h"
#include "dreplfs.h"

/*
 * The inode cache is used with alloc_inode for both our inode info and the
 * vfs inode.
 */
static struct kmem_cache *dreplfs_inode_cachep;

/* final actions when unmounting a file system */
static void dreplfs_put_super(struct super_block *sb)
{
	struct dreplfs_sb_info *spd;

	spd = DREPLFS_SB(sb);
	if (!spd)
		return;

	kfree(spd);
	sb->s_fs_info = NULL;
}

/*
 * @flags: numeric mount options
 * @options: mount options string
 */
static int dreplfs_remount_fs(struct super_block *sb, int *flags, char *options)
{
	int err = 0;

	/*
	 * The VFS will take care of "ro" and "rw" flags among others.  We
	 * can safely accept a few flags (RDONLY, MANDLOCK), and honor
	 * SILENT, but anything else left over is an error.
	 */
	if ((*flags & ~(MS_RDONLY | MS_MANDLOCK | MS_SILENT)) != 0) {
		printk(KERN_ERR
		       "dreplfs: remount flags 0x%x unsupported\n", *flags);
		err = -EINVAL;
	}

	return err;
}

/*
 * Called by iput() when the inode reference count reached zero
 * and the inode is not hashed anywhere.  Used to clear anything
 * that needs to be, before the inode is completely destroyed and put
 * on the inode free list.
 */
static void dreplfs_evict_inode(struct inode *inode)
{
//	printk(KERN_DEFAULT "dreplfs_evict_inode\n");
	truncate_inode_pages(&inode->i_data, 0);
	clear_inode(inode);
	filemap_fdatawrite(inode->i_mapping);
//	end_writeback(inode);
}

static struct inode *dreplfs_alloc_inode(struct super_block *sb)
{
	struct dreplfs_inode_info *i;

//	printk(KERN_DEFAULT "dreplfs_alloc_inode\n");
	i = kmem_cache_alloc(dreplfs_inode_cachep, GFP_KERNEL);
	if (!i)
		return NULL;

	/* memset everything up to the inode to 0 */
	memset(i, 0, offsetof(struct dreplfs_inode_info, vfs_inode));

	i->vfs_inode.i_version = 1;
	return &i->vfs_inode;
}

static void dreplfs_destroy_inode(struct inode *inode)
{
//	printk(KERN_DEFAULT "dreplfs_destroy_inode\n");
	kmem_cache_free(dreplfs_inode_cachep, DREPLFS_I(inode));
}

/* dreplfs inode cache constructor */
static void init_once(void *obj)
{
	struct dreplfs_inode_info *i = obj;

	inode_init_once(&i->vfs_inode);
}

int dreplfs_init_inode_cache(void)
{
	int err = 0;

	dreplfs_inode_cachep =
		kmem_cache_create("dreplfs_inode_cache",
				  sizeof(struct dreplfs_inode_info), 0,
				  SLAB_RECLAIM_ACCOUNT, init_once);
	if (!dreplfs_inode_cachep)
		err = -ENOMEM;
	return err;
}

/* dreplfs inode cache destructor */
void dreplfs_destroy_inode_cache(void)
{
	if (dreplfs_inode_cachep)
		kmem_cache_destroy(dreplfs_inode_cachep);
}

/*
 * Used only in nfs, to kill any pending RPC tasks, so that subsequent
 * code can actually succeed and won't leave tasks that need handling.
 */
static void dreplfs_umount_begin(struct super_block *sb)
{
/*
	struct super_block *lower_sb;

	lower_sb = dreplfs_lower_super(sb);
	if (lower_sb && lower_sb->s_op && lower_sb->s_op->umount_begin)
		lower_sb->s_op->umount_begin(lower_sb);
*/
}

const struct super_operations dreplfs_sops = {
	.put_super	= dreplfs_put_super,
	.statfs		= simple_statfs,
	.remount_fs	= dreplfs_remount_fs,
	.evict_inode	= dreplfs_evict_inode,
	.umount_begin	= dreplfs_umount_begin,
	.show_options	= generic_show_options,
	.alloc_inode	= dreplfs_alloc_inode,
	.destroy_inode	= dreplfs_destroy_inode,
	.drop_inode	= generic_delete_inode,
};
