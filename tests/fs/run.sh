#!/bin/bash

SIZES="1 2 4 8 16 64 128 170"
FLUSHDEV=/dev/sdc
MEMSZ=8192
FSPATH=/home/lucho/work/drepl/tests/fs

rm -f results-*
for sz in `echo $SIZES` ; do
	SZ=`expr $sz '*' 1048576`
	for fp in '1 2 3' '1 2' '1 3' '2 3' '1' '2' ; do
		arg=''
		rprefix=''
		for i in `echo $fp` ; do
			arg="$arg -v$i /mnt/ssd/v$i"
			rprefix="$rprefix$i"
		done

		rm -f /mnt/ssd/v1 /mnt/ssd/v2 /mnt/ssd/v3
		echo $FSPATH/dsfs -len=$SZ $arg
		$FSPATH/dsfs -len=$SZ $arg &
		sleep 5
		mount -t 9p 127.0.0.1 /mnt/9 -o port=5640,msize=3276
		for op in w R W r b B; do
			echo $sz $op
			# flush buffer cache
			dd if=$FLUSHDEV of=/dev/null bs=1048576 count=8192 2>/dev/null

			# run the test
			$FSPATH/dscl -m /mnt/9 $op >> $FSPATH/results-$rprefix
		done
		umount /mnt/9
	done
done
