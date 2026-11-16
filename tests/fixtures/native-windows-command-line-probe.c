/* Original Gestalt test fixture. SPDX-License-Identifier: MIT.
 * Appended after the byte-exact production C source with wmain renamed.
 * --vector uses the SAME run_owned_windows/atomic Job/terminal/result code;
 * its test-only file transport bypasses only the outer Node argv ceiling. */
int wmain(int argc, wchar_t **argv) {
  if (argc > 1 && !wcscmp(argv[1], L"--echo")) {
    wchar_t filename[32768];
    if (!GetEnvironmentVariableW(L"GESTALT_TEST_ARGV_REPORT", filename, 32768)) return 31;
    FILE *report = _wfopen(filename, L"wb"); if (!report) return 32;
    test_put32(report, 0x41524756); test_put32(report, (uint32_t)argc);
    for (unsigned int i = 0; i < 3; i++) {
      DWORD mode; HANDLE fd = GetStdHandle(i == 0 ? STD_INPUT_HANDLE : i == 1 ? STD_OUTPUT_HANDLE : STD_ERROR_HANDLE);
      test_put32(report, GetConsoleMode(fd, &mode) && _isatty((int)i));
    }
    BOOL member = FALSE; if (!IsProcessInJob(GetCurrentProcess(), NULL, &member)) return 33;
    test_put32(report, member); test_put_wide(report, GetCommandLineW());
    for (int i = 0; i < argc; i++) test_put_wide(report, argv[i]);
    if (fclose(report)) return 34;
    fputs("ARGV_CAPTURED\n", stdout); return 23;
  }
  if (argc != 4 || wcscmp(argv[1], L"--vector") ||
      (wcscmp(argv[3], L"byte") && wcscmp(argv[3], L"terminal"))) return 35;
  FILE *file = _wfopen(argv[2], L"rb"); if (!file) return 36;
  int target_argc = 0; wchar_t **target = test_vector(file, &target_argc); fclose(file); if (!target) return 37;
  InitializeCriticalSection(&output_lock);
  control_input = GetStdHandle(STD_INPUT_HANDLE); control_output = GetStdHandle(STD_OUTPUT_HANDLE);
  if (!wcscmp(argv[3], L"terminal")) { terminal_columns = 91; terminal_rows = 37; }
  uint32_t size; if (read_frame(&size) != 'L' || size) return 38;
  int result = run_owned_windows(target_argc, target, 50);
  test_free_vector(target_argc, target); return result;
}
