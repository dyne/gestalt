/* Original diagnostic fixture. SPDX-License-Identifier: MIT */
#define _GNU_SOURCE
#include <errno.h>
#include <linux/sched.h>
#include <pthread.h>
#include <sched.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ptrace.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#include <unistd.h>
static void hold(void){
 const char *value=getenv("GESTALT_PTRACE_FIXTURE_RELEASE_FD");
 if(value){int fd=atoi(value);char c;while(read(fd,&c,1)<0&&errno==EINTR){} _exit(0);}
 for(;;)pause();
}
static void *thread(void *p){(void)p;pid_t pid=fork();if(!pid)_exit(0);int s;if(waitpid(pid,&s,0)!=pid||s)_exit(87);return NULL;}
static void *exec_thread(void *p){(void)p;char *args[]={"/bin/true",NULL};execv(args[0],args);_exit(88);}
int main(int argc,char **argv){
 setbuf(stdout,NULL);if(argc<2)return 2;
 if(!strcmp(argv[1],"probes")){
  errno=0;long u=syscall(SYS_clone,(unsigned long)(CLONE_UNTRACED|SIGCHLD),0,0,0,0);int ue=errno;if(!u)_exit(81);if(u>0)waitpid((pid_t)u,NULL,0);
  struct clone_args args;memset(&args,0,sizeof(args));args.flags=CLONE_UNTRACED;args.exit_signal=SIGCHLD;
  errno=0;long c=syscall(SYS_clone3,&args,sizeof(args));int ce=errno;if(!c)_exit(82);if(c>0)waitpid((pid_t)c,NULL,0);
#ifdef __x86_64__
  errno=0;long x=syscall(0x40000000|SYS_getpid);int xe=errno;
#else
  long x=-1;int xe=ENOSYS;
#endif
  printf("{\"untraced\":%ld,\"untracedErrno\":%d,\"clone3\":%ld,\"clone3Errno\":%d,\"x32\":%ld,\"x32Errno\":%d}\n",u,ue,c,ce,x,xe);
  pthread_t workers[4];for(int i=0;i<4;i++)if(pthread_create(&workers[i],NULL,thread,NULL))return 83;for(int i=0;i<4;i++)pthread_join(workers[i],NULL);
  pid_t v=vfork();if(!v)_exit(0);int status;if(waitpid(v,&status,0)!=v||status)return 84;
  pid_t n=fork();if(!n){errno=0;long t=ptrace(PTRACE_TRACEME,0,0,0);printf("{\"nestedTracer\":%ld,\"errno\":%d}\n",t,errno);_exit(t<0&&errno==EPERM?0:86);}if(waitpid(n,&status,0)!=n||status)return 85;
  return u==-1&&ue==EPERM&&c==-1&&ce==ENOSYS&&x==-1&&xe==ENOSYS?23:89;
 }
 if(!strcmp(argv[1],"thread-exec")){pthread_t t;if(pthread_create(&t,NULL,exec_thread,NULL))return 90;hold();}
 if(!strcmp(argv[1],"tree")){
  int ready[2];if(pipe(ready))return 91;pid_t worker=fork();if(worker<0)return 92;
  if(!worker){close(ready[0]);if(setsid()<0)_exit(93);signal(SIGTERM,SIG_IGN);pid_t leaf=fork();if(leaf<0)_exit(94);
   if(!leaf){if(setsid()<0)_exit(95);signal(SIGTERM,SIG_IGN);printf("{\"kind\":\"leaf\",\"pid\":%d}\n",getpid());if(write(ready[1],"L",1)!=1)_exit(96);hold();}
   printf("{\"kind\":\"worker\",\"pid\":%d}\n",getpid());hold();}
  close(ready[1]);char c;if(read(ready[0],&c,1)!=1)return 97;
  printf("{\"kind\":\"root\",\"pid\":%d}\n{\"kind\":\"ready\"}\n",getpid());hold();
 }
 return 2;
}
