/* Diagnostic prototype only. Original MIT-licensed work, Gestalt contributors. */
#define _GNU_SOURCE
#include <errno.h>
#include <linux/audit.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <sched.h>
#include <signal.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/prctl.h>
#include <sys/ptrace.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#include <unistd.h>

static int filter(void) {
#if defined(__x86_64__)
#define OWN_ARCH AUDIT_ARCH_X86_64
#elif defined(__aarch64__)
#define OWN_ARCH AUDIT_ARCH_AARCH64
#else
 errno=ENOTSUP; return -1;
#endif
 struct sock_filter code[]={
  BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,arch)),
  BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,OWN_ARCH,1,0),
  BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_KILL_PROCESS),
  BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,nr)),
#if defined(__x86_64__)
  BPF_JUMP(BPF_JMP|BPF_JSET|BPF_K,0x40000000,0,1),
  BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|ENOSYS),
#endif
  BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,SYS_clone3,0,1),
  BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|ENOSYS),
  BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,SYS_clone,0,3),
  BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,args[0])),
  BPF_JUMP(BPF_JMP|BPF_JSET|BPF_K,CLONE_UNTRACED,0,1),
  BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|EPERM),
  BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ALLOW)
 };
 struct sock_fprog p={sizeof(code)/sizeof(code[0]),code};
 if(prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)<0) return -1;
 return prctl(PR_SET_SECCOMP,SECCOMP_MODE_FILTER,&p);
}
int main(void){
 setbuf(stdout,NULL);
 int ready[2],gate[2]; if(pipe(ready)||pipe(gate))return 2;
 pid_t owner=getpid(),child=fork(); if(child<0)return 2;
 if(!child){
  close(ready[0]);close(gate[1]);
  if(prctl(PR_SET_PDEATHSIG,SIGKILL)||getppid()!=owner)_exit(91);
  int e=filter()<0?errno:0; if(write(ready[1],&e,sizeof(e))!=sizeof(e))_exit(92);
  char c; if(e||read(gate[0],&c,1)!=1)_exit(93);
  /* Payload marker can only execute after SEIZE+INTERRUPT succeeded. */
  puts("PAYLOAD_STARTED");_exit(23);
 }
 close(ready[1]);close(gate[0]);
 int e=0,status=0; if(read(ready[0],&e,sizeof(e))!=sizeof(e))e=EIO;
 printf("filter_errno=%d child=%d uid=%d arch=%s\n",e,child,getuid(),
#if defined(__x86_64__)
 "x86_64"
#else
 "aarch64"
#endif
 );
 unsigned long opts=PTRACE_O_EXITKILL|PTRACE_O_TRACEFORK|PTRACE_O_TRACEVFORK|PTRACE_O_TRACECLONE|PTRACE_O_TRACEEXEC|PTRACE_O_TRACEEXIT;
 int result=0;
 if(e||ptrace(PTRACE_SEIZE,child,0,opts)<0){
  printf("seize_errno=%d payload_started=false\n",e?e:errno);result=77;
 }else if(ptrace(PTRACE_INTERRUPT,child,0,0)<0||waitpid(child,&status,__WALL)!=child||!WIFSTOPPED(status)){
  printf("interrupt_errno=%d status=%d payload_started=false\n",errno,status);result=78;
 }else{
  printf("attached_options=%lu initial_event=%d\n",opts,status>>16);
  if(write(gate[1],"G",1)!=1||ptrace(PTRACE_CONT,child,0,0)<0)result=79;
  while(!result&&waitpid(child,&status,__WALL)==child){
   if(WIFEXITED(status)){printf("payload_exit=%d\n",WEXITSTATUS(status));break;}
   if(WIFSIGNALED(status)){printf("payload_signal=%d\n",WTERMSIG(status));result=80;break;}
   if(ptrace(PTRACE_CONT,child,0,0)<0){result=81;break;}
  }
 }
 if(result){kill(child,SIGKILL);while(waitpid(child,&status,__WALL)>0){if(WIFEXITED(status)||WIFSIGNALED(status))break;ptrace(PTRACE_CONT,child,0,SIGKILL);}}
 close(gate[1]);close(ready[0]);printf("diagnostic_exit=%d child_settled=true\n",result);return result;
}
