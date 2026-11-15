/* Original diagnostic fixture. SPDX-License-Identifier: MIT
 * Copyright (c) 2026 Dyne.org contributors.
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
 * of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to inclusion of this notice in all copies/substantial portions.
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM,
 * DAMAGES OR OTHER LIABILITY ARISING FROM THE SOFTWARE OR ITS USE.
 */
#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0A00
#include <windows.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <wchar.h>
#include <io.h>
#pragma comment(lib, "advapi32.lib")

struct descriptor { HANDLE handle; DWORD type, type_error, console_mode, console_error; BOOL console; int crt_tty; intptr_t crt_handle; };
struct dimensions { BOOL ok; DWORD error; int columns, rows, buffer_columns, buffer_rows; };
static struct descriptor descriptors[3];
static struct dimensions initial, resized;
static WCHAR input[16]; static DWORD input_count, input_error;
static BOOL input_ok, mode_set, in_job;
static DWORD mode_error, elevated, elevation_type, token_error;
static BOOL admin;

static struct dimensions size(void) {
  struct dimensions result = {0}; CONSOLE_SCREEN_BUFFER_INFO info;
  SetLastError(0); result.ok = GetConsoleScreenBufferInfo(GetStdHandle(STD_OUTPUT_HANDLE), &info); result.error = GetLastError();
  if (result.ok) { result.columns = info.srWindow.Right - info.srWindow.Left + 1; result.rows = info.srWindow.Bottom - info.srWindow.Top + 1; result.buffer_columns = info.dwSize.X; result.buffer_rows = info.dwSize.Y; }
  return result;
}
static void inspect(void) {
  DWORD names[3] = {STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE};
  for (int i = 0; i < 3; i++) {
    descriptors[i].handle = GetStdHandle(names[i]); SetLastError(0);
    descriptors[i].type = GetFileType(descriptors[i].handle); descriptors[i].type_error = GetLastError(); SetLastError(0);
    descriptors[i].console = GetConsoleMode(descriptors[i].handle, &descriptors[i].console_mode); descriptors[i].console_error = GetLastError();
    descriptors[i].crt_tty = _isatty(i); descriptors[i].crt_handle = _get_osfhandle(i);
  }
  HANDLE token; DWORD bytes;
  if (OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
    TOKEN_ELEVATION elevation; TOKEN_ELEVATION_TYPE type;
    if (GetTokenInformation(token, TokenElevation, &elevation, sizeof(elevation), &bytes)) elevated = elevation.TokenIsElevated; else token_error = GetLastError();
    if (GetTokenInformation(token, TokenElevationType, &type, sizeof(type), &bytes)) elevation_type = type; else token_error = GetLastError();
    CloseHandle(token);
  } else token_error = GetLastError();
  SID_IDENTIFIER_AUTHORITY nt = SECURITY_NT_AUTHORITY; PSID admins = NULL;
  if (AllocateAndInitializeSid(&nt, 2, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS, 0, 0, 0, 0, 0, 0, &admins)) { CheckTokenMembership(NULL, admins, &admin); FreeSid(admins); }
  IsProcessInJob(GetCurrentProcess(), NULL, &in_job);
  initial = size();
}
static int report(const WCHAR *path, const char *phase) {
  HANDLE file = CreateFileW(path, GENERIC_WRITE, FILE_SHARE_READ, NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
  if (file == INVALID_HANDLE_VALUE) return -1;
  char json[4096]; int offset = snprintf(json, sizeof(json), "{\"kind\":\"native-win32-stdio\",\"pid\":%lu,\"phase\":\"%s\",\"elevated\":%lu,\"elevationType\":%lu,\"admin\":%s,\"tokenError\":%lu,\"inSomeJob\":%s,\"descriptors\":[", (unsigned long)GetCurrentProcessId(), phase, (unsigned long)elevated, (unsigned long)elevation_type, admin ? "true" : "false", (unsigned long)token_error, in_job ? "true" : "false");
  for (int i = 0; i < 3; i++) {
    struct descriptor *d = &descriptors[i];
    offset += snprintf(json + offset, sizeof(json) - (size_t)offset, "%s{\"fd\":%d,\"handle\":\"%p\",\"type\":%lu,\"typeError\":%lu,\"console\":%s,\"consoleMode\":%lu,\"consoleError\":%lu,\"crtIsatty\":%d,\"crtHandle\":\"%llx\"}", i ? "," : "", i, (void *)d->handle, (unsigned long)d->type, (unsigned long)d->type_error, d->console ? "true" : "false", (unsigned long)d->console_mode, (unsigned long)d->console_error, d->crt_tty, (unsigned long long)d->crt_handle);
  }
  offset += snprintf(json + offset, sizeof(json) - (size_t)offset, "],\"initialSize\":{\"ok\":%s,\"error\":%lu,\"columns\":%d,\"rows\":%d},\"resized\":{\"ok\":%s,\"error\":%lu,\"columns\":%d,\"rows\":%d},\"modeSet\":%s,\"modeError\":%lu,\"inputOk\":%s,\"inputError\":%lu,\"inputCodeUnits\":[", initial.ok ? "true" : "false", (unsigned long)initial.error, initial.columns, initial.rows, resized.ok ? "true" : "false", (unsigned long)resized.error, resized.columns, resized.rows, mode_set ? "true" : "false", (unsigned long)mode_error, input_ok ? "true" : "false", (unsigned long)input_error);
  for (DWORD i = 0; i < input_count; i++) offset += snprintf(json + offset, sizeof(json) - (size_t)offset, "%s%u", i ? "," : "", (unsigned int)input[i]);
  offset += snprintf(json + offset, sizeof(json) - (size_t)offset, "]}\n");
  if (offset <= 0 || (size_t)offset >= sizeof(json)) { CloseHandle(file); return -1; }
  DWORD written = 0; BOOL ok = WriteFile(file, json, (DWORD)offset, &written, NULL); FlushFileBuffers(file); CloseHandle(file); return ok && written == (DWORD)offset ? 0 : -1;
}
int wmain(int argc, wchar_t **argv) {
  if (argc != 3) return 91;
  inspect(); if (report(argv[1], "initial")) return 92;
  DWORD written; const char *out = "TARGET_STDOUT_READY\r\n", *err = "TARGET_STDERR_READY\r\n";
  WriteFile(GetStdHandle(STD_OUTPUT_HANDLE), out, (DWORD)strlen(out), &written, NULL);
  WriteFile(GetStdHandle(STD_ERROR_HANDLE), err, (DWORD)strlen(err), &written, NULL);
  if (!wcscmp(argv[2], L"terminal")) {
    DWORD mode = descriptors[0].console_mode & ~(ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT | ENABLE_PROCESSED_INPUT);
    SetLastError(0); mode_set = SetConsoleMode(GetStdHandle(STD_INPUT_HANDLE), mode | ENABLE_VIRTUAL_TERMINAL_INPUT); mode_error = GetLastError();
    if (report(argv[1], "ready")) return 92;
    while (mode_set && input_count < 4) {
      DWORD count = 0; SetLastError(0); input_ok = ReadConsoleW(GetStdHandle(STD_INPUT_HANDLE), input + input_count, 4 - input_count, &count, NULL); input_error = GetLastError();
      if (!input_ok || !count) break;
      input_count += count;
    }
  } else {
    unsigned char bytes[9]; DWORD offset = 0;
    while (offset < sizeof(bytes)) { DWORD count; if (!ReadFile(GetStdHandle(STD_INPUT_HANDLE), bytes + offset, (DWORD)sizeof(bytes) - offset, &count, NULL) || !count) break; offset += count; }
    input_count = (DWORD)MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, (const char *)bytes, (int)offset, input, 16); input_ok = input_count == 4;
  }
  resized = size(); if (report(argv[1], "input-complete")) return 93;
  return input_ok && input_count == 4 && input[0] == 233 && input[1] == 26481 && input[2] == 20140 && input[3] == 4 ? 23 : 94;
}
