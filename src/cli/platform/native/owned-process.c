/* Original Gestalt implementation. SPDX-License-Identifier: MIT
 * Copyright (c) 2026 Dyne.org contributors.
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
 * of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to the following conditions: The above copyright notice and this
 * permission notice shall be included in all copies or substantial portions of
 * the Software. THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
 * EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
 * OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
 * ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
 * DEALINGS IN THE SOFTWARE.
 */
#define _GNU_SOURCE
#ifndef _DARWIN_C_SOURCE
#define _DARWIN_C_SOURCE
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <errno.h>
#include <limits.h>
#include <wchar.h>
#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0A00
#include <windows.h>
#include <io.h>
#include <fcntl.h>
#else
#include <unistd.h>
#include <fcntl.h>
#include <signal.h>
#include <poll.h>
#include <sys/wait.h>
#include <time.h>
#ifdef __linux__
#include <sched.h>
#include <sys/prctl.h>
#include <sys/mount.h>
#include <sys/stat.h>
#elif defined(__APPLE__)
#include <spawn.h>
#endif
#endif

/* Binary protocol: little-endian uint32 length, byte kind, payload. No shell,
 * JSON parser, executable source interpolation, PID-based recovery or TCP.
 * L commit launch, I stdin bytes, E stdin EOF, S stop; R ready, O stdout,
 * B stderr, F root result, D proven empty, U best-effort unknown, X error.
 */
#define MAX_FRAME (1024U * 1024U)
static unsigned char payload[MAX_FRAME];
static int frame(unsigned char kind, const void *data, uint32_t size);
#ifdef _WIN32
static CRITICAL_SECTION output_lock;
static HANDLE control_input, control_output;
static int write_all(HANDLE fd, const void *data, uint32_t size) {
  const char *p = data;
  while (size) { DWORD n; if (!WriteFile(fd, p, size, &n, NULL) || !n) return -1; p += n; size -= n; }
  return 0;
}
static int read_all(HANDLE fd, void *data, uint32_t size) {
  char *p = data;
  while (size) { DWORD n; if (!ReadFile(fd, p, size, &n, NULL) || !n) return -1; p += n; size -= n; }
  return 0;
}
#else
static int write_all(int fd, const void *data, uint32_t size) {
  const char *p = data;
  while (size) { ssize_t n = write(fd, p, size); if (n < 0 && errno == EINTR) continue; if (n <= 0) return -1; p += n; size -= (uint32_t)n; }
  return 0;
}
static int read_all(int fd, void *data, uint32_t size) {
  char *p = data;
  while (size) { ssize_t n = read(fd, p, size); if (n < 0 && errno == EINTR) continue; if (n <= 0) return -1; p += n; size -= (uint32_t)n; }
  return 0;
}
#endif
static int frame(unsigned char kind, const void *data, uint32_t size) {
  unsigned char header[5]; uint32_t len = size + 1;
  for (unsigned int i = 0; i < 4; i++) header[i] = (unsigned char)(len >> (8 * i));
  header[4] = kind;
#ifdef _WIN32
  EnterCriticalSection(&output_lock);
  int result = write_all(control_output, header, 5);
  if (!result && size) result = write_all(control_output, data, size);
  LeaveCriticalSection(&output_lock);
#else
  int result = write_all(1, header, 5);
  if (!result && size) result = write_all(1, data, size);
#endif
  return result;
}
static int message(unsigned char kind, const char *text) { return frame(kind, text, (uint32_t)strlen(text)); }
static int read_frame(uint32_t *size) {
  unsigned char h[4];
#ifdef _WIN32
  HANDLE input = control_input;
#else
  int input = 0;
#endif
  if (read_all(input, h, 4)) return -1;
  uint32_t n = (uint32_t)h[0] | (uint32_t)h[1] << 8 | (uint32_t)h[2] << 16 | (uint32_t)h[3] << 24;
  if (!n || n > MAX_FRAME || read_all(input, payload, n)) return -1;
  *size = n - 1;
  return payload[0];
}

#ifndef _WIN32
static uint64_t millis(void) {
  struct timespec t; if (clock_gettime(CLOCK_MONOTONIC, &t)) return 0;
  return (uint64_t)t.tv_sec * 1000 + (uint64_t)t.tv_nsec / 1000000;
}
#endif
#ifdef __linux__
static int mapping(const char *file, unsigned int id) {
  char data[96]; int size = snprintf(data, sizeof(data), "%u %u 1\n", id, id);
  int fd = open(file, O_WRONLY | O_CLOEXEC); if (fd < 0) return -1;
  int result = write_all(fd, data, (uint32_t)size); int error = errno; close(fd); errno = error; return result;
}
static int namespace_error(const char *stage, int error, unsigned int uid, unsigned int gid) {
  struct stat info; int known = stat("/proc/self/setgroups", &info) == 0;
  char state[16] = "unreadable", policy[128] = "unreadable", caps[32] = "unreadable", text[900];
  int fd = open("/proc/self/setgroups", O_RDONLY | O_CLOEXEC);
  if (fd >= 0) { ssize_t n = read(fd, state, sizeof(state) - 1); if (n >= 0) state[n] = 0; close(fd); }
  state[strcspn(state, "\r\n")] = 0;
  fd = open("/proc/self/attr/current", O_RDONLY | O_CLOEXEC);
  if (fd >= 0) { ssize_t n = read(fd, policy, sizeof(policy) - 1); if (n >= 0) policy[n] = 0; close(fd); }
  policy[strcspn(policy, "\r\n")] = 0;
  FILE *status = fopen("/proc/self/status", "r");
  if (status) {
    char line[256];
    while (fgets(line, sizeof(line), status)) if (strncmp(line, "CapEff:", 7) == 0) {
      (void)sscanf(line + 7, "%31s", caps); break;
    }
    fclose(status);
  }
  snprintf(text, sizeof(text), "Namespace setup refused: stage=%s errno=%d (%s) requested-uid=%u requested-gid=%u euid=%u egid=%u dumpable=%d setgroups-owner=%ld:%ld mode=%lo state=%s CapEff=%s policy=%s",
      stage, error, strerror(error), uid, gid, (unsigned int)geteuid(), (unsigned int)getegid(), prctl(PR_GET_DUMPABLE),
      known ? (long)info.st_uid : -1, known ? (long)info.st_gid : -1, known ? (unsigned long)(info.st_mode & 0777) : 0, state, caps, policy);
  message('X', text); return 2;
}
static void graceful(int sig) { (void)sig; (void)kill(-1, SIGTERM); }
static void init_process(char **argv, int in[2], int out[2], int err[2], int life[2], int status[2], int ready[2]) {
  close(life[1]); close(status[0]); close(ready[0]);
  /* Parent might have died before prctl. No target is created until both the
   * kernel death signal and the private parent-liveness pipe are established. */
  if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0) _exit(110);
  struct pollfd parent = { life[0], POLLIN | POLLHUP, 0 };
  if (poll(&parent, 1, 0) < 0 || (parent.revents & POLLHUP)) _exit(111);
  if (mount(NULL, "/", NULL, MS_REC | MS_PRIVATE, NULL) || mount("proc", "/proc", "proc", MS_NOSUID | MS_NOEXEC | MS_NODEV, NULL)) _exit(112);
  struct sigaction action; memset(&action, 0, sizeof(action)); action.sa_handler = graceful; sigemptyset(&action.sa_mask);
  if (sigaction(SIGUSR1, &action, NULL)) _exit(113);
  int launch[2]; if (pipe2(launch, O_CLOEXEC)) _exit(114);
  pid_t root = fork();
  if (root < 0) _exit(115);
  if (!root) {
    close(launch[0]);
    if (dup2(in[0], 0) < 0 || dup2(out[1], 1) < 0 || dup2(err[1], 2) < 0) _exit(116);
    signal(SIGPIPE, SIG_DFL);
    /* All nonstandard descriptors are CLOEXEC; none of the lifetime/control
     * writers can reach the managed executable or its descendants. */
    execv(argv[0], argv);
    int error = errno; (void)write_all(launch[1], &error, sizeof(error)); _exit(127);
  }
  close(launch[1]); close(in[0]); close(in[1]); close(out[0]); close(out[1]); close(err[0]); close(err[1]);
  int error = 0; ssize_t n;
  do { n = read(launch[0], &error, sizeof(error)); } while (n < 0 && errno == EINTR);
  close(launch[0]);
  if (n != 0) { (void)write_all(ready[1], &error, sizeof(error)); _exit(117); }
  error = 0; if (write_all(ready[1], &error, sizeof(error))) _exit(118); close(ready[1]);
  int result; pid_t waited;
  do { waited = waitpid(root, &result, 0); } while (waited < 0 && errno == EINTR);
  if (waited != root) _exit(119);
  if (write_all(status[1], &result, sizeof(result))) _exit(120);
  /* Exiting PID-namespace init invokes zap_pid_ns_processes: allocation is
   * disabled, remaining members killed and waited for before init is reaped. */
  _exit(0);
}
static int run_owned(char **argv, unsigned int grace, pid_t controller) {
  if (prctl(PR_SET_PDEATHSIG, SIGKILL) || getppid() != controller) return 2;
  if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)) return 2;
  unsigned int uid = getuid(), gid = getgid();
  if (unshare(CLONE_NEWUSER | CLONE_NEWNS)) return namespace_error("unshare-user-mount", errno, uid, gid);
  int groups = open("/proc/self/setgroups", O_WRONLY | O_CLOEXEC);
  if (groups < 0) return namespace_error("setgroups-open", errno, uid, gid);
  if (write_all(groups, "deny", 4)) { int error = errno; close(groups); return namespace_error("setgroups-write", error, uid, gid); }
  close(groups);
  if (mapping("/proc/self/uid_map", uid)) return namespace_error("uid-map", errno, uid, gid);
  if (mapping("/proc/self/gid_map", gid)) return namespace_error("gid-map", errno, uid, gid);
  if (unshare(CLONE_NEWPID)) return namespace_error("unshare-pid", errno, uid, gid);
  int in[2], out[2], err[2], life[2], status[2], ready[2];
  if (pipe2(in, O_CLOEXEC) || pipe2(out, O_CLOEXEC) || pipe2(err, O_CLOEXEC) || pipe2(life, O_CLOEXEC) || pipe2(status, O_CLOEXEC) || pipe2(ready, O_CLOEXEC)) return 2;
  pid_t init = fork(); if (init < 0) return 2;
  if (!init) init_process(argv, in, out, err, life, status, ready);
  close(in[0]); close(out[1]); close(err[1]); close(life[0]); close(status[1]); close(ready[1]);
  /* This direct child PID cannot be reused until our exact waitpid reaps it. */
  int start_error = -1;
  if (read_all(ready[0], &start_error, sizeof(start_error)) || start_error) {
    (void)kill(init, SIGKILL); while (waitpid(init, NULL, 0) < 0 && errno == EINTR) {}
    message('F', "spawn-error:Native launch failed before readiness"); message('D', "linux-pid-namespace"); return 0;
  }
  close(ready[0]); message('R', "linux-pid-namespace");
  int stopping = 0, reaped = 0, input_open = 1, output_open = 1, error_open = 1, control_open = 1, root_result = 0, have_result = 0;
  uint64_t deadline = 0;
  while (!reaped || output_open || error_open) {
    struct pollfd fds[3] = {{control_open ? 0 : -1, POLLIN | POLLHUP, 0}, {output_open ? out[0] : -1, POLLIN | POLLHUP, 0}, {error_open ? err[0] : -1, POLLIN | POLLHUP, 0}};
    if (poll(fds, 3, 10) < 0 && errno != EINTR) { message('X', "Ownership control poll failed"); break; }
    if (fds[0].revents & (POLLIN | POLLHUP)) {
      uint32_t size; int kind = read_frame(&size);
      if (kind == 'I' && input_open) { if (write_all(in[1], payload + 1, size)) { close(in[1]); input_open = 0; } }
      else if (kind == 'E' && input_open) { close(in[1]); input_open = 0; }
      else if (kind == 'S' || kind < 0) {
        if (!stopping && !reaped) { (void)kill(init, SIGUSR1); deadline = millis() + (kind < 0 ? 0 : grace); stopping = 1; }
        if (kind < 0) control_open = 0;
      } else if (kind != 'I' && kind != 'E') { message('X', "Invalid ownership control frame"); stopping = 1; deadline = 0; }
    }
    for (int i = 1; i < 3; i++) if (fds[i].revents & (POLLIN | POLLHUP)) {
      unsigned char bytes[32768]; ssize_t n = read(fds[i].fd, bytes, sizeof(bytes));
      if (n > 0) { if (frame(i == 1 ? 'O' : 'B', bytes, (uint32_t)n)) { (void)kill(init, SIGKILL); } }
      else if (n == 0) { close(fds[i].fd); if (i == 1) output_open = 0; else error_open = 0; }
      else if (errno != EINTR) { message('X', "Owned stream read failed"); break; }
    }
    if (!reaped && stopping && millis() >= deadline) (void)kill(init, SIGKILL);
    if (!reaped) {
      pid_t waited = waitpid(init, NULL, WNOHANG);
      if (waited == init) {
        reaped = 1; close(life[1]); if (input_open) { close(in[1]); input_open = 0; }
        have_result = read_all(status[0], &root_result, sizeof(root_result)) == 0;
      } else if (waited < 0 && errno != EINTR) { message('X', "Exact namespace-init wait failed"); break; }
    }
  }
  if (!reaped) { (void)kill(init, SIGKILL); while (waitpid(init, NULL, 0) < 0 && errno == EINTR) {} return 2; }
  char text[80];
  if (have_result && WIFEXITED(root_result)) snprintf(text, sizeof(text), "exit:%d", WEXITSTATUS(root_result));
  else if (have_result && WIFSIGNALED(root_result)) snprintf(text, sizeof(text), "signal:%d", WTERMSIG(root_result));
  else snprintf(text, sizeof(text), "signal:9");
  message('F', text); message('D', "linux-pid-namespace"); return 0;
}
#elif defined(__APPLE__)
extern char **environ;
static int mac_pipe(int pair[2]) {
  if (pipe(pair)) return -1;
  if (fcntl(pair[0], F_SETFD, FD_CLOEXEC) || fcntl(pair[1], F_SETFD, FD_CLOEXEC)) {
    close(pair[0]); close(pair[1]); return -1;
  }
  return 0;
}
static void mac_pause(unsigned int ms) {
  struct timespec remaining = { ms / 1000, (long)(ms % 1000) * 1000000 };
  while (nanosleep(&remaining, &remaining) && errno == EINTR) {}
}
/* This direct child pins the private process-group identity until reaped.
 * Its pipe has exactly one writer, in the supervisor. A supervisor crash
 * therefore also attempts bounded cleanup, without claiming kernel containment. */
static void mac_anchor(int life[2], int ready[2], unsigned int grace) {
  close(life[1]); close(ready[0]);
  signal(SIGTERM, SIG_IGN); signal(SIGINT, SIG_IGN);
  int error = setpgid(0, 0) ? errno : 0;
  if (write_all(ready[1], &error, sizeof(error)) || error) _exit(121);
  close(ready[1]); close(0); close(1); close(2);
  char byte; ssize_t n;
  do { n = read(life[0], &byte, 1); } while (n < 0 && errno == EINTR);
  /* The anchor is itself still a member of this private group; no lookup or
   * stale PID recovery is involved. Detached descendants remain outside it. */
  (void)kill(0, SIGTERM); mac_pause(grace); (void)kill(0, SIGKILL); _exit(122);
}
static int mac_live_anchor(pid_t anchor, int *reaped) {
  if (*reaped) return 0;
  pid_t state = waitpid(anchor, NULL, WNOHANG);
  if (state == anchor) { *reaped = 1; return 0; }
  if (state != 0) return 0;
  return getpgid(anchor) == anchor && getsid(anchor) == getpid();
}
static int mac_signal_group(pid_t anchor, int *reaped, int signal_number) {
  if (!mac_live_anchor(anchor, reaped)) return -1;
  /* Even if the anchor exits here, it remains our unreaped direct child:
   * its PID cannot be reused for a replacement group before this signal. */
  return kill(-anchor, signal_number);
}
static int run_owned(char **argv, unsigned int grace, pid_t controller) {
  if (getppid() != controller || setsid() < 0) {
    message('F', "spawn-error:Private macOS session could not be established");
    message('U', "macOS session unavailable; retain scratch"); return 0;
  }
  int life[2], ready[2];
  if (mac_pipe(life) || mac_pipe(ready)) return 2;
  pid_t anchor = fork(); if (anchor < 0) return 2;
  if (!anchor) mac_anchor(life, ready, grace);
  close(life[0]); close(ready[1]);
  int reaped = 0, start_error = -1;
  if (read_all(ready[0], &start_error, sizeof(start_error)) || start_error || !mac_live_anchor(anchor, &reaped)) {
    if (!reaped) { (void)kill(anchor, SIGKILL); while (waitpid(anchor, NULL, 0) < 0 && errno == EINTR) {} }
    message('F', "spawn-error:Live macOS group anchor unavailable"); message('U', "macOS anchor unavailable; retain scratch"); return 0;
  }
  close(ready[0]);
  int in[2], out[2], err[2];
  if (mac_pipe(in) || mac_pipe(out) || mac_pipe(err)) return 2;
  posix_spawnattr_t attributes; posix_spawn_file_actions_t actions;
  if (posix_spawnattr_init(&attributes) || posix_spawn_file_actions_init(&actions)) return 2;
  sigset_t defaults, mask; sigemptyset(&defaults); sigemptyset(&mask);
  sigaddset(&defaults, SIGTERM); sigaddset(&defaults, SIGINT); sigaddset(&defaults, SIGPIPE); sigaddset(&defaults, SIGHUP); sigaddset(&defaults, SIGQUIT);
  short flags = POSIX_SPAWN_SETPGROUP | POSIX_SPAWN_SETSIGDEF | POSIX_SPAWN_SETSIGMASK;
#ifdef POSIX_SPAWN_CLOEXEC_DEFAULT
  flags |= POSIX_SPAWN_CLOEXEC_DEFAULT;
#endif
  int setup = posix_spawnattr_setflags(&attributes, flags) || posix_spawnattr_setpgroup(&attributes, anchor) ||
      posix_spawnattr_setsigdefault(&attributes, &defaults) || posix_spawnattr_setsigmask(&attributes, &mask) ||
      posix_spawn_file_actions_adddup2(&actions, in[0], 0) || posix_spawn_file_actions_adddup2(&actions, out[1], 1) ||
      posix_spawn_file_actions_adddup2(&actions, err[1], 2) || posix_spawn_file_actions_addclose(&actions, life[1]);
  pid_t root = -1;
  int spawned = setup ? EINVAL : posix_spawn(&root, argv[0], &actions, &attributes, argv, environ);
  posix_spawn_file_actions_destroy(&actions); posix_spawnattr_destroy(&attributes);
  close(in[0]); close(out[1]); close(err[1]);
  if (spawned) {
    (void)mac_signal_group(anchor, &reaped, SIGKILL);
    if (!reaped) while (waitpid(anchor, NULL, 0) < 0 && errno == EINTR) {}
    char reason[160]; snprintf(reason, sizeof(reason), "spawn-error:Native macOS launch failed (%d: %s)", spawned, strerror(spawned));
    message('F', reason); message('U', "macOS launch failed; complete descendant absence is unknown; retain scratch"); return 0;
  }
  message('R', "macos-session-group");
  int stopping = 0, forced = 0, root_reaped = 0, input_open = 1, output_open = 1, error_open = 1, control_open = 1, result_sent = 0, anchor_lost = 0;
  uint64_t deadline = 0, drain_deadline = 0;
  while (1) {
    if (!root_reaped) {
      int status; pid_t waited = waitpid(root, &status, WNOHANG);
      if (waited == root) {
        char result[64]; root_reaped = 1;
        if (WIFEXITED(status)) snprintf(result, sizeof(result), "exit:%d", WEXITSTATUS(status));
        else if (WIFSIGNALED(status)) snprintf(result, sizeof(result), "signal:%d", WTERMSIG(status));
        else { message('U', "macOS command status unavailable; retain scratch"); break; }
        message('F', result); result_sent = 1;
      } else if (waited < 0 && errno != EINTR) { message('U', "macOS command wait failed; retain scratch"); break; }
    }
    if (root_reaped && !stopping) {
      if (mac_signal_group(anchor, &reaped, SIGTERM)) anchor_lost = 1;
      stopping = 1; deadline = millis() + grace;
    }
    if (!forced && !anchor_lost && !mac_live_anchor(anchor, &reaped)) anchor_lost = 1;
    if (anchor_lost) { message('U', "macOS live group anchor lost; no stale group signal; retain scratch"); break; }
    if (stopping && !forced && millis() >= deadline) {
      if (mac_signal_group(anchor, &reaped, SIGKILL)) { message('U', "macOS group escalation could not be verified; retain scratch"); break; }
      forced = 1; drain_deadline = millis() + 1000;
    }
    if (forced) {
      if (!reaped) { pid_t waited = waitpid(anchor, NULL, WNOHANG); if (waited == anchor) reaped = 1; }
      if ((root_reaped && reaped && !output_open && !error_open) || millis() >= drain_deadline) break;
    }
    struct pollfd fds[3] = {{control_open ? 0 : -1, POLLIN | POLLHUP, 0}, {output_open ? out[0] : -1, POLLIN | POLLHUP, 0}, {error_open ? err[0] : -1, POLLIN | POLLHUP, 0}};
    if (poll(fds, 3, 10) < 0 && errno != EINTR) { message('U', "macOS group supervision poll failed; retain scratch"); break; }
    if (fds[0].revents & (POLLIN | POLLHUP)) {
      uint32_t size; int kind = read_frame(&size);
      if (kind == 'I' && input_open) { if (write_all(in[1], payload + 1, size)) { close(in[1]); input_open = 0; } }
      else if (kind == 'E' && input_open) { close(in[1]); input_open = 0; }
      else if (kind == 'S' || kind < 0) {
        if (!stopping) {
          if (mac_signal_group(anchor, &reaped, SIGTERM)) anchor_lost = 1;
          stopping = 1; deadline = millis() + grace;
        }
        if (kind < 0) control_open = 0;
      } else if (kind != 'I' && kind != 'E') { message('U', "macOS control channel invalid; retain scratch"); break; }
    }
    for (int i = 1; i < 3; i++) if (fds[i].revents & (POLLIN | POLLHUP)) {
      unsigned char bytes[32768]; ssize_t n = read(fds[i].fd, bytes, sizeof(bytes));
      if (n > 0) { if (frame(i == 1 ? 'O' : 'B', bytes, (uint32_t)n)) { stopping = 1; deadline = 0; } }
      else if (n == 0) { close(fds[i].fd); if (i == 1) output_open = 0; else error_open = 0; }
      else if (errno != EINTR) { message('U', "macOS stream inspection failed; retain scratch"); anchor_lost = 1; }
    }
  }
  if (input_open) close(in[1]); if (output_open) close(out[0]); if (error_open) close(err[0]);
  /* EOF also makes a surviving anchor attempt bounded group cleanup. A helper
   * or anchor crash still has no kernel whole-descendant guarantee on macOS. */
  close(life[1]);
  if (!result_sent) message('U', "macOS command result unavailable after bounded cleanup; retain scratch");
  message('U', "macOS best-effort group cleanup ended; detached descendants may survive; complete absence unknown; retain scratch");
  return 0;
}
#else
static volatile LONG output_failed = 0;
struct pump { HANDLE source; unsigned char kind; };
static DWORD WINAPI pump_output(void *argument) {
  struct pump *p = argument; unsigned char bytes[32768]; DWORD count;
  while (ReadFile(p->source, bytes, sizeof(bytes), &count, NULL) && count)
    if (frame(p->kind, bytes, count)) { InterlockedExchange(&output_failed, 1); break; }
  return 0;
}
static int active_processes(HANDLE job, DWORD *count) {
  JOBOBJECT_BASIC_ACCOUNTING_INFORMATION info;
  if (!QueryInformationJobObject(job, JobObjectBasicAccountingInformation, &info, sizeof(info), NULL)) return -1;
  *count = info.ActiveProcesses; return 0;
}
static wchar_t *command_line(int argc, wchar_t **argv) {
  size_t needed = 1;
  for (int i = 0; i < argc; i++) needed += wcslen(argv[i]) * 2 + 4;
  if (needed > 32767) return NULL;
  wchar_t *line = calloc(needed, sizeof(wchar_t)); if (!line) return NULL;
  wchar_t *p = line;
  for (int i = 0; i < argc; i++) {
    if (i) *p++ = L' '; *p++ = L'"';
    size_t slashes = 0;
    for (const wchar_t *s = argv[i]; ; s++) {
      if (*s == L'\\') { slashes++; continue; }
      size_t copies = slashes * ((*s == L'"' || !*s) ? 2 : 1);
      for (size_t n = 0; n < copies; n++) *p++ = L'\\';
      slashes = 0;
      if (!*s) break;
      if (*s == L'"') *p++ = L'\\';
      *p++ = *s;
    }
    *p++ = L'"';
  }
  *p = 0; return line;
}
static int run_owned_windows(int argc, wchar_t **argv, unsigned int grace) {
  HANDLE job = CreateJobObjectW(NULL, NULL); if (!job) return 2;
  JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits; memset(&limits, 0, sizeof(limits));
  /* Neither BREAKAWAY_OK nor SILENT_BREAKAWAY_OK. No name and no inherited
   * job handle: controller EOF or helper crash closes the last ownership handle. */
  limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
  if (!SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits))) { CloseHandle(job); return 2; }
  SECURITY_ATTRIBUTES security = { sizeof(security), NULL, TRUE };
  HANDLE input_read = NULL, input_write = NULL, output_read = NULL, output_write = NULL, error_read = NULL, error_write = NULL;
  if (!CreatePipe(&input_read, &input_write, &security, 0) || !CreatePipe(&output_read, &output_write, &security, 0) || !CreatePipe(&error_read, &error_write, &security, 0)) { CloseHandle(job); return 2; }
  if (!SetHandleInformation(input_write, HANDLE_FLAG_INHERIT, 0) || !SetHandleInformation(output_read, HANDLE_FLAG_INHERIT, 0) || !SetHandleInformation(error_read, HANDLE_FLAG_INHERIT, 0)) { CloseHandle(job); return 2; }
  SIZE_T size = 0; InitializeProcThreadAttributeList(NULL, 2, 0, &size);
  LPPROC_THREAD_ATTRIBUTE_LIST attributes = malloc(size);
  HANDLE inherited[3] = {input_read, output_write, error_write};
  if (!attributes || !InitializeProcThreadAttributeList(attributes, 2, 0, &size) ||
      !UpdateProcThreadAttribute(attributes, 0, PROC_THREAD_ATTRIBUTE_JOB_LIST, &job, sizeof(job), NULL, NULL) ||
      !UpdateProcThreadAttribute(attributes, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST, inherited, sizeof(inherited), NULL, NULL)) {
    message('X', "Atomic Windows job assignment unavailable; no suspended-process fallback"); CloseHandle(job); return 2;
  }
  /* A private helper console permits best-effort CTRL_BREAK to the owned
   * process group without broadcasting to the controller or other processes. */
  FreeConsole(); BOOL console = AllocConsole();
  if (console) { HWND window = GetConsoleWindow(); if (window) ShowWindow(window, SW_HIDE); }
  STARTUPINFOEXW startup; memset(&startup, 0, sizeof(startup)); startup.StartupInfo.cb = sizeof(startup);
  startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES | STARTF_USESHOWWINDOW;
  startup.StartupInfo.wShowWindow = SW_HIDE; startup.StartupInfo.hStdInput = input_read;
  startup.StartupInfo.hStdOutput = output_write; startup.StartupInfo.hStdError = error_write; startup.lpAttributeList = attributes;
  PROCESS_INFORMATION process; memset(&process, 0, sizeof(process));
  wchar_t *line = command_line(argc, argv);
  BOOL created = line && CreateProcessW(argv[0], line, NULL, NULL, TRUE,
      EXTENDED_STARTUPINFO_PRESENT | CREATE_UNICODE_ENVIRONMENT | CREATE_NEW_PROCESS_GROUP, NULL, NULL, &startup.StartupInfo, &process);
  DWORD creation_error = created ? 0 : GetLastError(); free(line); DeleteProcThreadAttributeList(attributes); free(attributes);
  CloseHandle(input_read); CloseHandle(output_write); CloseHandle(error_write);
  if (!created) {
    char reason[128]; snprintf(reason, sizeof(reason), "spawn-error:Atomic job launch failed (%lu)", (unsigned long)creation_error); message('F', reason);
    DWORD active;
    if (!active_processes(job, &active) && active == 0) message('D', "windows-job");
    CloseHandle(job); return 0;
  }
  CloseHandle(process.hThread);
  BOOL member = FALSE;
  if (!IsProcessInJob(process.hProcess, job, &member) || !member) { CloseHandle(job); message('X', "Atomic job membership not established"); return 2; }
  struct pump output = { output_read, 'O' }, error = { error_read, 'B' };
  HANDLE pumps[2] = {CreateThread(NULL, 0, pump_output, &output, 0, NULL), CreateThread(NULL, 0, pump_output, &error, 0, NULL)};
  if (!pumps[0] || !pumps[1]) { CloseHandle(job); return 2; }
  message('R', "windows-job");
  int stopping = 0, root_exited = 0, proven = 0; ULONGLONG deadline = 0; DWORD root_code = 1;
  while (1) {
    DWORD available = 0; BOOL control = PeekNamedPipe(control_input, NULL, 0, NULL, &available, NULL);
    if (!control || available) {
      uint32_t size; int kind = control ? read_frame(&size) : -1;
      if (kind == 'I' && input_write) {
        if (write_all(input_write, payload + 1, size)) { CloseHandle(input_write); input_write = NULL; }
      } else if (kind == 'E' && input_write) { CloseHandle(input_write); input_write = NULL; }
      else if (kind == 'S' || kind < 0) {
        if (!stopping) {
          if (console && WaitForSingleObject(process.hProcess, 0) == WAIT_TIMEOUT) (void)GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, process.dwProcessId);
          stopping = 1; deadline = GetTickCount64() + (kind < 0 ? 0 : grace);
        }
      } else if (kind != 'I' && kind != 'E') { message('X', "Invalid ownership control frame"); break; }
    }
    if (!root_exited && WaitForSingleObject(process.hProcess, 0) == WAIT_OBJECT_0) {
      if (!GetExitCodeProcess(process.hProcess, &root_code)) { message('X', "Root handle result unavailable"); break; }
      root_exited = 1;
    }
    DWORD active;
    if (active_processes(job, &active)) { message('X', "Job active-process accounting unavailable"); break; }
    if (active == 0) { proven = 1; break; }
    if (root_exited || (stopping && GetTickCount64() >= deadline) || output_failed)
      if (!TerminateJobObject(job, 137)) { message('X', "Owned job termination failed"); break; }
    Sleep(10);
  }
  if (input_write) CloseHandle(input_write);
  if (!proven) { CloseHandle(job); return 2; }
  if (WaitForMultipleObjects(2, pumps, TRUE, 5000) != WAIT_OBJECT_0) { message('X', "Owned output did not settle after empty job proof"); CloseHandle(job); return 2; }
  if (!GetExitCodeProcess(process.hProcess, &root_code) || WaitForSingleObject(process.hProcess, 0) != WAIT_OBJECT_0 || output_failed) { CloseHandle(job); return 2; }
  CloseHandle(pumps[0]); CloseHandle(pumps[1]); CloseHandle(output_read); CloseHandle(error_read);
  char result[64]; snprintf(result, sizeof(result), "exit:%lu", (unsigned long)root_code); message('F', result);
  /* This handle identifies the same job created before launch, with breakaway
   * disabled. No PID scan, parent-exit inference or elapsed-time empty claim. */
  message('D', "windows-job"); CloseHandle(process.hProcess); CloseHandle(job); return 0;
}
#endif

#ifndef _WIN32
int main(int argc, char **argv) {
  if (argc < 7 || strcmp(argv[1], "--controller") || strcmp(argv[3], "--grace") || strcmp(argv[5], "--")) return 2;
  char *end; long controller = strtol(argv[2], &end, 10); if (*end || controller < 1 || controller > INT_MAX) return 2;
  long grace = strtol(argv[4], &end, 10); if (*end || grace < 0 || grace > 60000) return 2;
  signal(SIGPIPE, SIG_IGN);
  uint32_t size; if (read_frame(&size) != 'L' || size) return 2;
  return run_owned(argv + 6, (unsigned int)grace, (pid_t)controller);
}
#endif

#ifdef _WIN32
int wmain(int argc, wchar_t **argv) {
  InitializeCriticalSection(&output_lock);
  control_input = GetStdHandle(STD_INPUT_HANDLE); control_output = GetStdHandle(STD_OUTPUT_HANDLE);
  if (argc < 7 || wcscmp(argv[1], L"--controller") || wcscmp(argv[3], L"--grace") || wcscmp(argv[5], L"--")) return 2;
  wchar_t *end; long grace = wcstol(argv[4], &end, 10); if (*end || grace < 0 || grace > 60000) return 2;
  uint32_t size; if (read_frame(&size) != 'L' || size) return 2;
  return run_owned_windows(argc - 6, argv + 6, (unsigned int)grace);
}
#endif
