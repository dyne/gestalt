/* ABI/group-stop diagnostic only. SPDX-License-Identifier: MIT */
#define _GNU_SOURCE
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <sys/prctl.h>
#include <unistd.h>
#include <stddef.h>
#include <fcntl.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <sys/syscall.h>
int main(int argc,char **argv){
 setbuf(stdout,NULL);if(argc<2)return 2;
 if(!strcmp(argv[1],"deny-tracing")){
  if(argc<3)return 2;
  struct sock_filter code[]={
   BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,nr)),
   BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,SYS_ptrace,0,1),
   BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|EPERM),
   BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ALLOW)
  };
  struct sock_fprog program={sizeof(code)/sizeof(code[0]),code};
  if(prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)||prctl(PR_SET_SECCOMP,SECCOMP_MODE_FILTER,&program))return 89;
  execv(argv[2],argv+2);return 90;
 }
 if(!strcmp(argv[1],"marker")){
  if(argc!=3)return 2;
  int fd=open(argv[2],O_WRONLY|O_CREAT|O_EXCL,0600);if(fd<0)return 91;
  if(write(fd,"PAYLOAD_STARTED",15)!=15)return 92;
  close(fd);
  printf("PAYLOAD_STARTED nnp=%d\n",prctl(PR_GET_NO_NEW_PRIVS,0,0,0,0));
  FILE *status=fopen("/proc/self/status","r");if(!status)return 93;
  char line[256];while(fgets(line,sizeof(line),status))if(!strncmp(line,"TracerPid:",10))fputs(line,stdout);
  fclose(status);return 23;
 }
 if(!strcmp(argv[1],"compat")){
#if defined(__x86_64__)
  long result;__asm__ volatile("int $0x80":"=a"(result):"0"(20):"memory");
  printf("COMPAT_ESCAPED %ld\n",result);return 87;
#else
  return 77;
#endif
 }
 if(!strcmp(argv[1],"group-stop")){
  printf("STOPPING %d\n",getpid());raise(SIGSTOP);puts("RESUMED");return 23;
 }
 if(!strcmp(argv[1],"nnp")){
  int inherited=prctl(PR_GET_NO_NEW_PRIVS,0,0,0,0);errno=0;int removed=prctl(PR_SET_NO_NEW_PRIVS,0,0,0,0);int e=errno;
  printf("NNP inherited=%d remove=%d errno=%d\n",inherited,removed,e);return inherited==1&&removed==-1?23:88;
 }
 return 2;
}
