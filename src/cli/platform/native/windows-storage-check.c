/* Original Gestalt implementation. SPDX-License-Identifier: MIT
 * Read-only counterpart of windowsSystemScript's two conservative ACL checks.
 * No process launch, privilege change, write, interpreter or ownership claim.
 */
#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0A00
#include <windows.h>
#include <aclapi.h>
#include <sddl.h>
#include <lm.h>
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include "windows-storage-check.h"

#define MAX_MEMBERS 4096U
#define MAX_SECURITY_BYTES (1024U * 1024U)
struct principals {
  BYTE caller[SECURITY_MAX_SID_SIZE], system[SECURITY_MAX_SID_SIZE];
  BYTE admins[SECURITY_MAX_SID_SIZE], installer[SECURITY_MAX_SID_SIZE];
  BYTE members[MAX_MEMBERS][SECURITY_MAX_SID_SIZE];
  DWORD count, error;
  int loaded;
};

static int diagnostic(const char *stage, DWORD error, int index, int code) {
  char text[192];
  int size = snprintf(text, sizeof(text), "GESTALT_STORAGE_CHECK_V1 %s error=%lu index=%d\n", stage, (unsigned long)error, index);
  DWORD written;
  if (size > 0 && (size_t)size < sizeof(text))
    (void)WriteFile(GetStdHandle(STD_ERROR_HANDLE), text, (DWORD)size, &written, NULL);
  return code;
}
static int inside(const void *base, size_t size, const void *value, size_t needed) {
  uintptr_t start = (uintptr_t)base, pointer = (uintptr_t)value;
  return pointer >= start && pointer - start <= size && needed <= size - (pointer - start);
}
static int bounded_sid(const void *base, size_t size, PSID sid) {
  if (!inside(base, size, sid, 8)) return 0;
  BYTE count = ((const BYTE *)sid)[1];
  return count <= SID_MAX_SUB_AUTHORITIES && inside(base, size, sid, 8U + (size_t)count * 4U) && IsValidSid(sid);
}
static int copy_known_sid(BYTE *destination, const wchar_t *text) {
  PSID sid = NULL;
  if (!ConvertStringSidToSidW(text, &sid)) return 0;
  int ok = IsValidSid(sid) && GetLengthSid(sid) <= SECURITY_MAX_SID_SIZE && CopySid(SECURITY_MAX_SID_SIZE, destination, sid);
  LocalFree(sid); return ok;
}
static int initialize_principals(struct principals *p) {
  HANDLE token = NULL; DWORD size = 0; void *information = NULL; int ok = 0;
  if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return 0;
  (void)GetTokenInformation(token, TokenUser, NULL, 0, &size);
  if (GetLastError() != ERROR_INSUFFICIENT_BUFFER || size < sizeof(TOKEN_USER) || size > 65536U) goto done;
  information = calloc(1, size);
  if (!information || !GetTokenInformation(token, TokenUser, information, size, &size)) goto done;
  PSID user = ((TOKEN_USER *)information)->User.Sid;
  if (!bounded_sid(information, size, user) || !CopySid(SECURITY_MAX_SID_SIZE, p->caller, user)) goto done;
  if (!copy_known_sid(p->system, L"S-1-5-18") || !copy_known_sid(p->admins, L"S-1-5-32-544") ||
      !copy_known_sid(p->installer, L"S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464")) goto done;
  ok = 1;
done:
  free(information); CloseHandle(token); return ok;
}
static int load_admin_users(struct principals *p) {
  wchar_t name[257], domain[257]; DWORD names = 257, domains = 257; SID_NAME_USE kind;
  if (!LookupAccountSidW(NULL, p->admins, name, &names, domain, &domains, &kind) || kind != SidTypeAlias) {
    p->error = GetLastError(); return 0;
  }
  DWORD_PTR resume = 0; DWORD total_seen = 0;
  for (unsigned int page = 0; page < 128; page++) {
    BYTE *buffer = NULL; DWORD read = 0, total = 0, bytes = 0; DWORD_PTR previous = resume;
    NET_API_STATUS status = NetLocalGroupGetMembers(NULL, name, 2, &buffer, 65536, &read, &total, &resume);
    int valid = (status == NERR_Success || status == ERROR_MORE_DATA) &&
      total_seen <= MAX_MEMBERS && read <= MAX_MEMBERS - total_seen &&
      buffer && NetApiBufferSize(buffer, &bytes) == NERR_Success && bytes <= MAX_SECURITY_BYTES &&
      (size_t)read <= bytes / sizeof(LOCALGROUP_MEMBERS_INFO_2);
    if (read == 0 && status == NERR_Success) valid = 1;
    if (valid && read) {
      LOCALGROUP_MEMBERS_INFO_2 *members = (LOCALGROUP_MEMBERS_INFO_2 *)buffer;
      for (DWORD i = 0; i < read; i++) {
        if (!bounded_sid(buffer, bytes, members[i].lgrmi2_sid)) { valid = 0; break; }
        SID_NAME_USE type = members[i].lgrmi2_sidusage;
        if (type == SidTypeUser) {
          if (p->count >= MAX_MEMBERS || !CopySid(SECURITY_MAX_SID_SIZE, p->members[p->count], members[i].lgrmi2_sid)) { valid = 0; break; }
          p->count++;
        } else if (type == SidTypeUnknown || type == SidTypeDeletedAccount || type < SidTypeUser || type > SidTypeLogonSession) {
          valid = 0; break;
        }
      }
    }
    if (buffer) NetApiBufferFree(buffer);
    if (!valid) { p->error = status == NERR_Success || status == ERROR_MORE_DATA ? ERROR_INVALID_DATA : status; return 0; }
    total_seen += read;
    if (status == NERR_Success) { p->loaded = 1; return 1; }
    if (!read || resume == previous) { p->error = ERROR_INVALID_DATA; return 0; }
  }
  p->error = ERROR_BUFFER_OVERFLOW; return 0;
}
static int trusted(struct principals *p, PSID sid) {
  if (EqualSid(sid, p->caller) || EqualSid(sid, p->system) || EqualSid(sid, p->admins) || EqualSid(sid, p->installer)) return 1;
  SID_IDENTIFIER_AUTHORITY *authority = GetSidIdentifierAuthority(sid);
  BYTE *count = GetSidSubAuthorityCount(sid);
  if (!count || !*count) return 0;
  DWORD first = *GetSidSubAuthority(sid, 0);
  static const BYTE nt[6] = {0,0,0,0,0,5}, cloud[6] = {0,0,0,0,0,12};
  if (!((!memcmp(authority->Value, nt, 6) && first == 21) || (!memcmp(authority->Value, cloud, 6) && first == 1))) return 0;
  if (!p->loaded && !load_admin_users(p)) return -1;
  for (DWORD i = 0; i < p->count; i++) if (EqualSid(sid, p->members[i])) return 1;
  return 0;
}
static int valid_path(const wchar_t *path) {
  size_t length = wcslen(path), start;
  if (length < 7 || length > 32766 || wcsncmp(path, L"\\\\?\\", 4)) return 0;
  if (((path[4] >= L'A' && path[4] <= L'Z') || (path[4] >= L'a' && path[4] <= L'z')) && path[5] == L':' && path[6] == L'\\') start = 7;
  else if (!_wcsnicmp(path + 4, L"UNC\\", 4)) {
    const wchar_t *server = path + 8, *share = wcschr(server, L'\\');
    if (!share || share == server || !share[1] || share[1] == L'\\') return 0;
    start = 8;
  } else return 0;
  const wchar_t *component = path + start;
  for (size_t i = start; i <= length; i++) {
    wchar_t unit = path[i];
    if (unit && (unit < 32 || unit == 127 || wcschr(L"<>:\"|?*", unit))) return 0;
    if (!unit || unit == L'\\') {
      size_t count = (size_t)(path + i - component);
      if ((count == 1 && component[0] == L'.') || (count == 2 && component[0] == L'.' && component[1] == L'.')) return 0;
      if (!count && unit) return 0;
      component = path + i + 1;
    }
  }
  return 1;
}
static int check_path(struct principals *p, const wchar_t *path, int namespace_mode, DWORD *error) {
  HANDLE handle = CreateFileW(path, READ_CONTROL,
    FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING,
    FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT, NULL);
  if (handle == INVALID_HANDLE_VALUE) { *error = GetLastError(); return 0; }
  BY_HANDLE_FILE_INFORMATION attributes; PSECURITY_DESCRIPTOR descriptor = NULL; PSID owner = NULL; PACL acl = NULL;
  int ok = 0, full = 0;
  if (!GetFileInformationByHandle(handle, &attributes)) { *error = GetLastError(); goto done; }
  if ((attributes.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT) ||
      (namespace_mode && !(attributes.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY))) { *error = ERROR_REPARSE_TAG_INVALID; goto done; }
  *error = GetSecurityInfo(handle, SE_FILE_OBJECT, OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION, &owner, NULL, &acl, NULL, &descriptor);
  if (*error != ERROR_SUCCESS) goto done;
  if (!descriptor || !IsValidSecurityDescriptor(descriptor)) { *error = ERROR_INVALID_SECURITY_DESCR; goto done; }
  DWORD size = GetSecurityDescriptorLength(descriptor);
  if (size > MAX_SECURITY_BYTES || !bounded_sid(descriptor, size, owner) || !acl ||
      !inside(descriptor, size, acl, sizeof(ACL)) || !inside(descriptor, size, acl, acl->AclSize) || !IsValidAcl(acl)) { *error = ERROR_INVALID_ACL; goto done; }
  int owner_ok = namespace_mode ? trusted(p, owner) : EqualSid(owner, p->caller);
  if (owner_ok != 1) { *error = owner_ok < 0 ? p->error : ERROR_ACCESS_DENIED; goto done; }
  for (DWORD i = 0; i < acl->AceCount; i++) {
    void *value = NULL;
    if (!GetAce(acl, i, &value) || !inside(acl, acl->AclSize, value, sizeof(ACE_HEADER))) { *error = ERROR_INVALID_ACL; goto done; }
    ACE_HEADER *header = (ACE_HEADER *)value;
    if (!inside(acl, acl->AclSize, value, header->AceSize) || header->AceSize < offsetof(ACCESS_ALLOWED_ACE, SidStart) + 8 ||
        (header->AceFlags & ~0x1fU) || (header->AceType != ACCESS_ALLOWED_ACE_TYPE && header->AceType != ACCESS_DENIED_ACE_TYPE)) { *error = ERROR_NOT_SUPPORTED; goto done; }
    ACCESS_ALLOWED_ACE *ace = (ACCESS_ALLOWED_ACE *)value; PSID sid = &ace->SidStart;
    if (!bounded_sid(value, header->AceSize, sid)) { *error = ERROR_INVALID_SID; goto done; }
    if (header->AceType == ACCESS_DENIED_ACE_TYPE) continue;
    if (namespace_mode) {
      if (header->AceFlags & INHERIT_ONLY_ACE) continue;
      DWORD mutating = DELETE | FILE_DELETE_CHILD | WRITE_DAC | WRITE_OWNER | FILE_WRITE_DATA | FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA;
      if (ace->Mask & mutating) {
        int accepted = trusted(p, sid);
        if (accepted != 1) { *error = accepted < 0 ? p->error : ERROR_ACCESS_DENIED; goto done; }
      }
    } else {
      if (!EqualSid(sid, p->caller) && !EqualSid(sid, p->system)) { *error = ERROR_ACCESS_DENIED; goto done; }
      if (EqualSid(sid, p->caller) && (ace->Mask & 2032127U) == 2032127U) full = 1;
    }
  }
  if (!namespace_mode && !full) { *error = ERROR_ACCESS_DENIED; goto done; }
  ok = 1;
done:
  if (descriptor) LocalFree(descriptor);
  CloseHandle(handle); return ok;
}
int gestalt_windows_storage_check(int argc, wchar_t **argv) {
  int namespace_mode = argc > 2 && !wcscmp(argv[2], L"namespace");
  if (argc < 5 || argc > 132 || wcscmp(argv[1], L"--storage-check-v1") || wcscmp(argv[3], L"--") ||
      (!namespace_mode && (wcscmp(argv[2], L"private") || argc != 5))) return diagnostic("request", ERROR_INVALID_PARAMETER, -1, 2);
  for (int i = 4; i < argc; i++) if (!valid_path(argv[i])) return diagnostic("path", ERROR_INVALID_NAME, i - 4, 2);
  struct principals *p = calloc(1, sizeof(*p));
  if (!p) return diagnostic("allocation", ERROR_NOT_ENOUGH_MEMORY, -1, 1);
  if (!initialize_principals(p)) { DWORD error = GetLastError(); free(p); return diagnostic("token", error, -1, 1); }
  for (int i = 4; i < argc; i++) {
    DWORD error = ERROR_INVALID_DATA;
    if (!check_path(p, argv[i], namespace_mode, &error)) { free(p); return diagnostic("guard", error, i - 4, 1); }
  }
  free(p);
  const char token[] = "GESTALT_STORAGE_CHECK_V1_OK\n"; DWORD written = 0;
  if (!WriteFile(GetStdHandle(STD_OUTPUT_HANDLE), token, (DWORD)(sizeof(token) - 1), &written, NULL) || written != sizeof(token) - 1) return 1;
  return 0;
}
#endif
