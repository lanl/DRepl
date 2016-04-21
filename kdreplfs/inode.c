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

static int dreplfs_inode_test(struct inode *inode, void *data)
{
	return 0;
}

static int dreplfs_inode_set(struct inode *inode, void *data)
{
	/* we do actual inode initialization in dreplfs_iget */
	return 0;
}

struct inode *dreplfs_iget(struct super_block *sb, drepl_view *v)
{
	struct inode *inode; /* the new inode to return */
	int err;
	struct dreplfs_sb_info *dsb;

//	printk(KERN_DEFAULT "dreplfs_iget view %p\n", v);
	dsb = DREPLFS_SB(sb);
	inode = iget5_locked(sb, v->id + 1, dreplfs_inode_test, dreplfs_inode_set, NULL);
	if (!inode) {
		err = -EACCES;
		return ERR_PTR(err);
	}
	/* if found a cached inode, then just return it */
	if (!(inode->i_state & I_NEW))
		return inode;

	/* initialize new inode */
	DREPLFS_I(inode)->view = v;
	
	inode->i_ino = v->id + 1;
	inode->i_version++;
	inode->i_op = &dreplfs_main_iops;
	inode->i_fop = &dreplfs_main_fops;

	inode->i_mapping->a_ops = &dreplfs_aops;
	inode->i_atime = CURRENT_TIME;
	inode->i_mtime = CURRENT_TIME;
	inode->i_ctime = CURRENT_TIME;
	inode->i_uid = dsb->uid;
	inode->i_gid = dsb->gid;
	inode->i_mode = S_IFREG | 0660;
	i_size_write(inode, v->size);
	inode->i_blocks = (i_size_read(inode) + 512 - 1) >> 9;
	if (v->repl && v->repl->file) {
		fsstack_copy_attr_times(inode, v->repl->file->f_path.dentry->d_inode);
	}

	unlock_new_inode(inode);
	return inode;
}

struct dentry *dreplfs_lookup(struct inode *dir, struct dentry *dentry,
				unsigned int flags)
{
	struct dentry *ret;
	struct dreplfs_sb_info *dsb;
	struct inode *inode;
	drepl *d;
	drepl_view *v;
	int i;
	char *name;

	dsb = DREPLFS_SB(dir->i_sb);
	d = dsb->d;
	name = (char *) dentry->d_name.name;

//	printk(KERN_DEFAULT "dreplfs_lookup %s\n", name);
	/* find the view */
	v = NULL;
	for(i = 0; i < d->nviews; i++) {
		if (strcmp(d->views[i].name, name) == 0) {
			v = &d->views[i];
			break;
		}
	}

	if (v) {
		inode = dreplfs_iget(dir->i_sb, v);
		if (IS_ERR(inode)) {
			ret = ERR_PTR(PTR_ERR(inode));
			inode = NULL;
			goto error;
		}

		ret = d_materialise_unique(dentry, inode);
		if (IS_ERR(ret)) {
			goto error;
		}
	} else {
		/* negative dentry */
		ret = d_materialise_unique(dentry, NULL);
	}

	return ret;

error:
	iput(inode);
	return ret;
}

const struct inode_operations dreplfs_dir_iops = {
	.lookup		= dreplfs_lookup,
};

const struct inode_operations dreplfs_main_iops = {
};
