import { Command } from 'commander';

const program = new Command();

program
  .name('gestalt-scip')
  .description('Query SCIP code intelligence indexes offline')
  .version('0.1.0');

program.parse();
