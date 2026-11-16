/* Bounded extra prototype fixtures. SPDX-License-Identifier: MIT */
#define main baseline_main
#include "fixture-base.c"
#undef main
static int token_fd,ready_fd;
static int held_clone(void *unused){
 (void)unused;signal(SIGTERM,SIG_IGN);if(setsid()<0)_exit(91);
 printf("{\"kind\":\"clone-parent\",\"pid\":%d}\n",getpid());
 if(write(ready_fd,"R",1)!=1)_exit(92);
 hold();return 0;
}
int main(int argc,char **argv){
 if(argc<3)return baseline_main(argc,argv);
 setbuf(stdout,NULL);token_fd=atoi(argv[2]);
 if(!strcmp(argv[1],"clone-parent")){
  int ready[2];if(pipe(ready))return 93;ready_fd=ready[1];char *stack=malloc(1024*1024);if(!stack)return 94;
  if(clone(held_clone,stack+1024*1024,CLONE_PARENT|SIGCHLD,NULL)<0)return 95;
  char c;if(read(ready[0],&c,1)!=1)return 96;
  printf("{\"kind\":\"root\",\"pid\":%d}\n{\"kind\":\"ready\"}\n",getpid());hold();
 }
 if(!strcmp(argv[1],"fork-race")){
  printf("{\"kind\":\"root\",\"pid\":%d}\n",getpid());
  for(int i=0;i<64;i++){
   pid_t child=fork();if(child<0)return 97;
   if(!child){signal(SIGTERM,SIG_IGN);if(setsid()<0)_exit(98);printf("{\"kind\":\"worker\",\"pid\":%d}\n",getpid());hold();}
  }hold();
 }
 return 2;
}
