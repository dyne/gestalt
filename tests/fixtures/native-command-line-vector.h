/* Original Gestalt test fixture. SPDX-License-Identifier: MIT. */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <wchar.h>
static int test_u32(FILE *file, uint32_t *value) {
  unsigned char b[4]; if (fread(b, 1, 4, file) != 4) return 0;
  *value = (uint32_t)b[0] | (uint32_t)b[1] << 8 | (uint32_t)b[2] << 16 | (uint32_t)b[3] << 24; return 1;
}
static void test_put32(FILE *file, uint32_t value) {
  unsigned char b[4] = { (unsigned char)value, (unsigned char)(value >> 8), (unsigned char)(value >> 16), (unsigned char)(value >> 24) };
  if (fwrite(b, 1, 4, file) != 4) abort();
}
static void test_put_wide(FILE *file, const wchar_t *value) {
  size_t n = wcslen(value); test_put32(file, (uint32_t)n);
  for (size_t i = 0; i < n; i++) {
    unsigned char b[2] = { (unsigned char)value[i], (unsigned char)((unsigned int)value[i] >> 8) };
    if (fwrite(b, 1, 2, file) != 2) abort();
  }
}
static void test_free_vector(int argc, wchar_t **argv) {
  for (int i = 0; i < argc; i++) free(argv[i]); free(argv);
}
static wchar_t **test_vector(FILE *file, int *argc) {
  uint32_t count, total = 0; if (!test_u32(file, &count) || !count || count > 32768) return NULL;
  wchar_t **argv = calloc(count, sizeof(*argv)); if (!argv) return NULL;
  *argc = (int)count;
  for (uint32_t i = 0; i < count; i++) {
    uint32_t n;
    if (!test_u32(file, &n) || n > 262144 || total > 262144 - n) goto failed;
    total += n; argv[i] = calloc((size_t)n + 1, sizeof(wchar_t)); if (!argv[i]) goto failed;
    for (uint32_t j = 0; j < n; j++) {
      unsigned char b[2]; if (fread(b, 1, 2, file) != 2) goto failed;
      argv[i][j] = (wchar_t)((unsigned int)b[0] | (unsigned int)b[1] << 8);
      if (!argv[i][j]) goto failed;
    }
  }
  return argv;
failed:
  test_free_vector(*argc, argv); return NULL;
}
