/* Original Gestalt test fixture. SPDX-License-Identifier: MIT.
 * Compiles the byte-exact Windows quoting block, not a rewritten algorithm.
 * Test inputs encode UTF16 code units explicitly even on wchar_t32 hosts. */
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <wchar.h>
#ifdef _WIN32
#include <windows.h>
#include <io.h>
#include <fcntl.h>
#else
static unsigned long test_error;
#define SetLastError(value) (test_error = (value))
#define GetLastError() test_error
#define ERROR_FILENAME_EXCED_RANGE 206
#define ERROR_NOT_ENOUGH_MEMORY 8
#define ERROR_INVALID_DATA 13
#endif
/* Preserve the accepted pre-fix block during RED compilation. */
#if defined(__GNUC__)
#pragma GCC diagnostic ignored "-Wmisleading-indentation"
#endif
/* QUOTING_BLOCK */
/* VECTOR_IO */
int main(void) {
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY); _setmode(_fileno(stdout), _O_BINARY);
#endif
  int argc = 0; wchar_t **argv = test_vector(stdin, &argc); if (!argv) return 2;
  SetLastError(0); wchar_t *line = command_line(argc, argv);
  test_put32(stdout, line ? 1 : 0); test_put32(stdout, (uint32_t)GetLastError());
  test_put_wide(stdout, line ? line : L"");
  free(line); test_free_vector(argc, argv); return ferror(stdout) ? 2 : 0;
}
