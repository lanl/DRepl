#include <unistd.h>
#include <stdlib.h>
#include <stdio.h>
#include <fcntl.h>
#include <sys/time.h>

#define set_time( time, tv_1, tv_2, tv_diff )  \
        timersub(&tv_2, &tv_1, &tv_diff); \
        *time = ((double)(tv_diff.tv_sec) * 1.0E+6 + \
                (double)(tv_diff.tv_usec)) 

#define timersub(a, b, result)                                                \
  do {                                                                        \
    (result)->tv_sec = (a)->tv_sec - (b)->tv_sec;                             \
    (result)->tv_usec = (a)->tv_usec - (b)->tv_usec;                          \
    if ((result)->tv_usec < 0) {                                              \
      --(result)->tv_sec;                                                     \
      (result)->tv_usec += 1000000;                                           \
    }                                                                         \
  } while (0)

#define ARRAY_SZ 10240*10240

struct float_struct{
float a,b,c;
}; 

write_arrays(int fd,int size, float a[],float b[],float c[]){
	write(fd, a, size * sizeof(float));
	write(fd, b, size * sizeof(float));
	write(fd, c, size * sizeof(float));
}
read_arrays(int fd,int size,float a[],float b[],float c[]){
	read(fd, a, size * sizeof(float));
	read(fd, b, size * sizeof(float));
	read(fd, c, size * sizeof(float));
	int i;
	//for(i=0;i<ARRAY_SZ;i++){ a[i] = 1.0; b[i]=2.0;c[i]=3.0; }
}
read_arrays_strided(int fd, int size, struct float_struct d[]){
	// read a block then assign to the structures
	float *bufa,*bufb,*bufc;
	int i;

	bufa = (float *)malloc (1024 * sizeof(float));
	bufb = (float *)malloc (1024 * sizeof(float));
	bufc = (float *)malloc (1024 * sizeof(float));
	int count=0;
	while ( count < size ){
	   read(fd,bufa, 1024 *  sizeof(float));

	   lseek(fd, (off_t) size + count, SEEK_SET); //seek to second array
	   read(fd,bufb, 1024 *  sizeof(float));

	   lseek(fd, (off_t) 2*size + count, SEEK_SET); //seek to third array
	   read(fd,bufc, 1024 *  sizeof(float));

	   for(i=0;i<1024;i++){		
		d[i+count].a = bufa[i];
		d[i+count].b = bufb[i];
		d[i+count].c = bufc[i];
	   }
	   count += 1024;
	   lseek(fd, (off_t) count, SEEK_SET); //back to first array + offset
	}
	//for(i=0;i<ARRAY_SZ;i++){ d[i].a = 1.5; d[i].b=2.5;d[i].c=3.5; }
}



main()
{
float a[ARRAY_SZ];
float b[ARRAY_SZ];
float c[ARRAY_SZ];

struct float_struct d[ARRAY_SZ];

struct timeval tv_1, tv_2, tv_diff;
double t;
int fd;
int size,fsize=ARRAY_SZ;



	fd = open("foo",O_RDWR|O_CREAT);
printf("size\twrite time\tread time\tread strided\n");
   for (size=1024; size<fsize; size *=2){
	printf("%d\t",size);

	lseek(fd, (off_t) 0, SEEK_SET); //rewind

	gettimeofday(&tv_1, NULL);
	write_arrays(fd,size, a,b,c); // write arrays sequentially
	gettimeofday(&tv_2, NULL);
	set_time( &t, tv_1, tv_2, tv_diff );
	printf("%f\t",t);

	lseek(fd, (off_t) 0, SEEK_SET); //rewind

	gettimeofday(&tv_1, NULL);
	read_arrays(fd,size,a,b,c); // read arrays sequentially
	gettimeofday(&tv_2, NULL);
	set_time( &t, tv_1, tv_2, tv_diff );
	printf("%f\t",t);

	lseek(fd, (off_t) 0, SEEK_SET); //rewind

	gettimeofday(&tv_1, NULL);
	read_arrays_strided(fd,size,d); // read the strided
	gettimeofday(&tv_2, NULL);
	set_time( &t, tv_1, tv_2, tv_diff );
	printf("%f\n",t);
   }
}
