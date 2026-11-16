/* Bounded diagnostic, not a production ownership backend. SPDX-License-Identifier: MIT */
#define main prerequisite_main
#include "preflight.c"
#undef main
#include <fcntl.h>
#include <poll.h>
#include <time.h>
#include <sys/ioctl.h>
#include <string.h>

struct task { pid_t tid; int live; };
static struct task tasks[1024];
static int count, stopping;
static int tracked(pid_t tid){for(int i=0;i<count;i++)if(tasks[i].live&&tasks[i].tid==tid)return i;return -1;}
static int add(pid_t tid){int i=tracked(tid);if(i>=0)return i;if(count==1024)return -1;tasks[count]=(struct task){tid,1};return count++;}
/* A traced thread stays waitable and its identity cannot be recycled before
 * this tracer consumes its final wait. There is no discovery/recovery by PID. */
static void stop_all(void){stopping=1;for(int i=0;i<count;i++)if(tasks[i].live)(void)syscall(SYS_tkill,tasks[i].tid,SIGKILL);}
static unsigned long long now(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return (unsigned long long)t.tv_sec*1000+t.tv_nsec/1000000;}
int main(int argc,char **argv){
 if(argc<2)return 2;
 int control_fd=0,terminal_fd=-1,target=1;
 if(argc>=7&&!strcmp(argv[1],"--terminal")){
  terminal_fd=atoi(argv[2]);control_fd=atoi(argv[4]);target=6;
  if(strcmp(argv[3],"--control")||strcmp(argv[5],"--")||terminal_fd<3||control_fd<3||terminal_fd==control_fd)return 2;
  if(fcntl(terminal_fd,F_SETFD,FD_CLOEXEC)||fcntl(control_fd,F_SETFD,FD_CLOEXEC))return 2;
 }
 setbuf(stderr,NULL);int ready[2],gate[2];if(pipe2(ready,O_CLOEXEC)||pipe2(gate,O_CLOEXEC))return 2;
 pid_t owner=getpid(),root=fork();if(root<0)return 2;
 if(!root){
  close(ready[0]);close(gate[1]);
  if(prctl(PR_SET_PDEATHSIG,SIGKILL)||getppid()!=owner)_exit(91);
  int e=filter()<0?errno:0;if(write(ready[1],&e,sizeof(e))!=sizeof(e))_exit(92);
  char c;if(e||read(gate[0],&c,1)!=1)_exit(93);
  close(gate[0]);close(ready[1]);
  if(terminal_fd>=0){
   if(setsid()<0||ioctl(terminal_fd,TIOCSCTTY,0)||dup2(terminal_fd,0)<0||dup2(terminal_fd,1)<0||dup2(terminal_fd,2)<0)_exit(94);
   close(terminal_fd);close(control_fd);
  }else{
   int input=open("/dev/null",O_RDONLY|O_CLOEXEC);if(input<0||dup2(input,0)<0)_exit(94);close(input);
  }
  execv(argv[target],argv+target);_exit(127);
 }
 close(ready[1]);close(gate[0]);int e=0,status;
 if(terminal_fd>=0)close(terminal_fd);
 if(read(ready[0],&e,sizeof(e))!=sizeof(e))e=EIO;
 unsigned long opts=PTRACE_O_EXITKILL|PTRACE_O_TRACEFORK|PTRACE_O_TRACEVFORK|PTRACE_O_TRACECLONE|PTRACE_O_TRACEEXEC|PTRACE_O_TRACEEXIT;
 if(e||ptrace(PTRACE_SEIZE,root,0,opts)||ptrace(PTRACE_INTERRUPT,root,0,0)||waitpid(root,&status,__WALL)!=root||!WIFSTOPPED(status)){
  fprintf(stderr,"PREREQUISITE_REFUSED errno=%d\n",e?e:errno);kill(root,SIGKILL);while(waitpid(root,&status,__WALL)>0){if(WIFEXITED(status)||WIFSIGNALED(status))break;ptrace(PTRACE_CONT,root,0,SIGKILL);}return 77;
 }
 if(add(root)<0||prctl(PR_SET_DUMPABLE,0))return 2;
 if(write(gate[1],"G",1)!=1||ptrace(PTRACE_CONT,root,0,0))return 2;
 close(gate[1]);close(ready[0]);fprintf(stderr,"TRACE_READY root=%d uid=%d\n",root,getuid());
 int root_status=-1,events=0,finals=0,unknown=0;unsigned long long deadline=now()+20000;
 for(;;){
  pid_t tid=waitpid(-1,&status,__WALL|WNOHANG);
  if(tid<0){if(errno==EINTR)continue;if(errno==ECHILD)break;unknown=1;break;}
  if(!tid){
   struct pollfd control={control_fd,POLLIN|POLLHUP,0};if(poll(&control,1,2)>0){char c;if(read(control_fd,&c,1)<=0||c=='S')stop_all();}
   if(now()>deadline){fprintf(stderr,"DEADLINE_UNKNOWN\n");unknown=1;stop_all();break;}continue;
  }
  int i=tracked(tid);if(i<0&&(i=add(tid))<0){unknown=1;break;}
  if(WIFEXITED(status)||WIFSIGNALED(status)){
   tasks[i].live=0;finals++;if(tid==root){root_status=status;stop_all();}continue;
  }
  if(!WIFSTOPPED(status)){unknown=1;break;}
  unsigned event=(unsigned)status>>16;int sig=WSTOPSIG(status);unsigned long msg=0;events++;
  if(event==PTRACE_EVENT_FORK||event==PTRACE_EVENT_VFORK||event==PTRACE_EVENT_CLONE){
   if(ptrace(PTRACE_GETEVENTMSG,tid,0,&msg)||add((pid_t)msg)<0){unknown=1;break;}
   fprintf(stderr,"TRACE_CHILD event=%u tid=%lu\n",event,msg);
   if(stopping)(void)syscall(SYS_tkill,(pid_t)msg,SIGKILL);
  }else if(event==PTRACE_EVENT_EXEC){
   if(ptrace(PTRACE_GETEVENTMSG,tid,0,&msg)){unknown=1;break;}
   if(msg!=(unsigned long)tid){int old=tracked((pid_t)msg);if(old>=0)tasks[old].live=0;}
   fprintf(stderr,"TRACE_EXEC tid=%d former=%lu\n",tid,msg);
  }
  if(event==PTRACE_EVENT_STOP&&(sig==SIGSTOP||sig==SIGTSTP||sig==SIGTTIN||sig==SIGTTOU)&&!stopping){
   if(ptrace(PTRACE_LISTEN,tid,0,0)){unknown=1;break;}
   fprintf(stderr,"LISTEN_READY tid=%d signal=%d result=0\n",tid,sig);continue;
  }
  if(ptrace(PTRACE_CONT,tid,0,event?0:sig)<0&&errno!=ESRCH){unknown=1;break;}
 }
 if(unknown){stop_all();fprintf(stderr,"PROTOTYPE_UNKNOWN tracked=%d finals=%d errno=%d\n",count,finals,errno);return 2;}
 int live=0;for(int i=0;i<count;i++)live+=tasks[i].live;
 fprintf(stderr,"TRACE_DRAINED tracked=%d finals=%d live=%d events=%d root_status=%d\n",count,finals,live,events,root_status);
 return root_status>=0&&!live?0:2;
}
