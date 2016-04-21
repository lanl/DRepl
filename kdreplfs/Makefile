DREPLFS_VERSION="0.1"
EXTRA_CFLAGS += -DDREPLFS_VERSION=\"$(DREPLFS_VERSION)\"
KPATH=/lib/modules/`uname -r`/build
obj-$(CONFIG_DREPL_FS) += dreplfs.o
obj-m += dreplfs.o

dreplfs-y := dentry.o file.o inode.o main.o super.o mmap.o import.o block.o view.o repl.o elo.o expr.o

ko:
	make -C $(KPATH) SUBDIRS=$(shell pwd) modules

ko-clean:
	make -C $(KPATH) SUBDIRS=$(shell pwd) clean
