import { Command, CommanderError } from 'commander';
import { definitionCommand } from './commands/definition.js';
import { filesCommand } from './commands/files.js';
import { referencesCommand } from './commands/references.js';
import { searchCommand } from './commands/search.js';
import { symbolsCommand } from './commands/symbols.js';

type AsyncCommand<TArgs extends unknown[]> = (...args: TArgs) => Promise<void>;

function enableBlockingOutput(): void {
  const stdoutHandle = (process.stdout as any)?._handle;
  stdoutHandle?.setBlocking?.(true);
  const stderrHandle = (process.stderr as any)?._handle;
  stderrHandle?.setBlocking?.(true);
}

function withErrorHandling<TArgs extends unknown[]>(command: AsyncCommand<TArgs>): AsyncCommand<TArgs> {
  return async (...args: TArgs) => {
    try {
      await command(...args);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error(`Error: ${message}`);
      process.exitCode = 1;
    }
  };
}

enableBlockingOutput();

export const program = new Command();

program
  .name('gestalt-scip')
  .description('Query SCIP code intelligence indexes offline')
  .version('0.1.0');

program
  .command('symbols <query>')
  .description('Search for symbols by name')
  .option('--scip <path>', 'Path to SCIP file or directory')
  .option('--language <lang>', 'Filter by language (go, typescript, python)')
  .option('--limit <n>', 'Max results (default: 20, max: 1000)', '20')
  .option('--format <fmt>', 'Output format (json|text|toon)', 'toon')
  .action(withErrorHandling(symbolsCommand));

program
  .command('definition <symbol-id>')
  .description('Get symbol definition by ID (base64url from symbols output)')
  .option('--scip <path>', 'Path to SCIP file or directory')
  .option('--format <fmt>', 'Output format (json|text|toon)', 'toon')
  .action(withErrorHandling(definitionCommand));

program
  .command('references <symbol-id>')
  .description('Get all references to symbol by ID (base64url from symbols output)')
  .option('--scip <path>', 'Path to SCIP file or directory')
  .option('--format <fmt>', 'Output format (json|text|toon)', 'toon')
  .action(withErrorHandling(referencesCommand));

program
  .command('files <path>')
  .description('Get file content with optional symbol annotations')
  .option('--scip <path>', 'Path to SCIP file or directory')
  .option('--format <fmt>', 'Output format (json|text|toon)', 'toon')
  .option('--symbols', 'Include symbol occurrences')
  .action(withErrorHandling(filesCommand));

program
  .command('search <pattern>')
  .description('Search file contents with regex patterns (supports OR via |)')
  .option('--scip <path>', 'Path to SCIP file or directory')
  .option('--language <lang>', 'Filter by language (go, typescript, python)')
  .option('--limit <n>', 'Max results (default: 50, max: 1000)', '50')
  .option('--format <fmt>', 'Output format (json|text|toon)', 'toon')
  .option('--case-sensitive', 'Enable case-sensitive search', false)
  .option('--context <n>', 'Lines of context (default: 2, max: 10)', '2')
  .action(withErrorHandling(searchCommand));

program.exitOverride();

export async function run(argv: string[] = process.argv): Promise<void> {
  try {
    await program.parseAsync(argv);
  } catch (error) {
    if (error instanceof CommanderError) {
      if (error.code === 'commander.helpDisplayed' || error.code === 'commander.version') {
        return;
      }
      process.exitCode = 1;
      return;
    }
    const message = error instanceof Error ? error.message : String(error);
    console.error(`Error: ${message}`);
    process.exitCode = 1;
  }
}

if (require.main === module) {
  void run();
}
