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
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <unistd.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <limits.h>
#ifdef __APPLE__
#include <sys/un.h>
#include <libproc.h>
#include <sys/acl.h>
#include <fcntl.h>
#include <membership.h>
#include <uuid/uuid.h>
#endif

#ifdef __APPLE__
static int private_acl(const char *path) {
  // Query the ACL property after a successful security-aware lstat. A missing
  // property is distinct from acl_get_file returning an ambiguous NULL/ENOENT.
  filesec_t security = filesec_init();
  if (!security) return 1;
  struct stat info; int present = 0, result = 1;
  acl_t acl = NULL;
  if (lstatx_np(path, &info, security) != 0 || info.st_uid != getuid() ||
      (info.st_mode & 077) != 0 || S_ISLNK(info.st_mode) ||
      (S_ISREG(info.st_mode) && info.st_nlink != 1) ||
      filesec_query_property(security, FILESEC_ACL, &present) != 0) goto done;
  if (!present) { result = 0; goto done; }
  if (filesec_get_property(security, FILESEC_ACL, &acl) != 0 || !acl) goto done;
  if (acl == (acl_t)_FILESEC_REMOVE_ACL) { acl = NULL; result = 0; goto done; }
  if (acl_valid(acl) != 0) goto done;
  uuid_t user_uuid, root_uuid;
  if (mbr_uid_to_uuid(getuid(), user_uuid) != 0 || mbr_uid_to_uuid(0, root_uuid) != 0) goto done;
  acl_entry_t entry;
  int entry_id = ACL_FIRST_ENTRY;
  while (acl_get_entry(acl, entry_id, &entry) == 0) {
    entry_id = ACL_NEXT_ENTRY;
    acl_tag_t tag;
    if (acl_get_tag_type(entry, &tag) != 0) goto done;
    if (tag == ACL_EXTENDED_DENY) continue;
    if (tag != ACL_EXTENDED_ALLOW) goto done;
    void *principal = acl_get_qualifier(entry);
    if (!principal) goto done;
    int trusted = memcmp(principal, user_uuid, sizeof(uuid_t)) == 0 || memcmp(principal, root_uuid, sizeof(uuid_t)) == 0;
    acl_free(principal);
    if (!trusted) goto done;
  }
  if (errno != EINVAL) goto done;
  result = 0;
done:
  if (acl) acl_free(acl);
  filesec_free(security);
  return result;
}
#endif

static int identity(pid_t pid, uid_t uid) {
  char executable[PATH_MAX + 1], started[96];
#ifdef __APPLE__
  struct proc_bsdinfo info;
  if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != (int)sizeof(info) ||
      info.pbi_uid != uid || proc_pidpath(pid, executable, sizeof(executable)) <= 0) return 1;
  snprintf(started, sizeof(started), "%llu.%06llu", (unsigned long long)info.pbi_start_tvsec,
      (unsigned long long)info.pbi_start_tvusec);
#else
  char filename[80], buffer[8192];
  struct stat process_stat;
  snprintf(filename, sizeof(filename), "/proc/%ld", (long)pid);
  if (stat(filename, &process_stat) != 0 || process_stat.st_uid != uid) return 1;
  snprintf(filename, sizeof(filename), "/proc/%ld/stat", (long)pid);
  FILE *file = fopen(filename, "r");
  if (!file) return 1;
  size_t count = fread(buffer, 1, sizeof(buffer) - 1, file);
  fclose(file); buffer[count] = 0;
  char *last = strrchr(buffer, ')');
  if (!last || last[1] != ' ') return 1;
  char *save = NULL, *token = strtok_r(last + 2, " ", &save);
  for (int field = 3; field < 22 && token; field++) token = strtok_r(NULL, " ", &save);
  if (!token || strlen(token) >= sizeof(started)) return 1;
  strcpy(started, token);
  snprintf(filename, sizeof(filename), "/proc/%ld/exe", (long)pid);
  ssize_t length = readlink(filename, executable, sizeof(executable) - 1);
  if (length < 1) return 1;
  executable[length] = 0;
#endif
  printf("{\"pid\":%ld,\"uid\":%lu,\"started\":\"%s\",\"executable\":\"", (long)pid, (unsigned long)uid, started);
  for (const unsigned char *p = (const unsigned char *)executable; *p; p++) {
    if (*p == '"' || *p == '\\') putchar('\\');
    if (*p < 32) printf("\\u%04x", *p); else putchar(*p);
  }
  puts("\"}"); return 0;
}

int main(int argc, char **argv) {
  pid_t pid; uid_t uid;
#ifdef __APPLE__
  if (argc == 3 && strcmp(argv[1], "--private") == 0) return private_acl(argv[2]);
#endif
  if (argc == 3 && strcmp(argv[1], "--identity") == 0) {
    char *end = NULL; errno = 0;
    long parsed = strtol(argv[2], &end, 10);
    if (errno || !end || *end || parsed < 1 || parsed > INT_MAX) return 2;
    return identity((pid_t)parsed, getuid());
  }
  if (argc != 2 || strcmp(argv[1], "--peer") != 0) return 2;
  // Descriptor 3 is inherited from the already-connected Node Socket. No reopen
  // by pathname, TCP, arbitrary fd scanning, or writable operation is involved.
#ifdef __APPLE__
  socklen_t size = sizeof(pid); gid_t gid;
  if (getsockopt(3, SOL_LOCAL, LOCAL_PEERPID, &pid, &size) != 0 || size != sizeof(pid) ||
      getpeereid(3, &uid, &gid) != 0) return 1;
#else
  struct ucred peer; socklen_t size = sizeof(peer);
  if (getsockopt(3, SOL_SOCKET, SO_PEERCRED, &peer, &size) != 0 || size != sizeof(peer)) return 1;
  pid = peer.pid; uid = peer.uid;
#endif
  if (uid != getuid() || pid <= 0) return 1;
  return identity(pid, uid);
}
