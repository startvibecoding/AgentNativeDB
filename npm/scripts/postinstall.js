#!/usr/bin/env node

// Skip postinstall output in CI or when suppressed
if (process.env.CI || process.env.npm_config_yes || process.env.ANDB_SKIP_POSTINSTALL) {
  process.exit(0);
}

const os = require('os');
const path = require('path');

const RESET  = '\x1b[0m';
const BOLD   = '\x1b[1m';
const DIM    = '\x1b[2m';
const CYAN   = '\x1b[36m';
const BRIGHT_GREEN = '\x1b[92m';
const WHITE  = '\x1b[97m';

const logo = [
  '    _        ___  ____  __  __',
  '   / \\   ___|  _ \\|  _ \\|  \\/  |',
  '  / _ \\ |_  / | | | | | | |\\/| |',
  ' / ___ \\ _\\/| |_| | |_| | |  | |',
  '/_/   \\_\\___|____/|____/|_|  |_|',
].join('\n');

function pkgVersion() {
  try {
    return require('../package.json').version;
  } catch {
    return '';
  }
}

const ver = pkgVersion();
const verStr = ver ? ` ${DIM}v${ver}${RESET}` : '';

console.log();
console.log(`${BRIGHT_GREEN}${BOLD}${logo}${RESET}${verStr}`);
console.log();
console.log(`  ${BOLD}${WHITE}Agent-Native Database — SQL + Vector + Graph + MCP${RESET}`);
console.log();
console.log(`  ${DIM}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}`);
console.log();
console.log(`  ${BOLD}Quick Start${RESET}`);
console.log();
console.log(`    andb server              ${DIM}Start HTTP API server${RESET}`);
console.log(`    andb cli                 ${DIM}Interactive SQL REPL${RESET}`);
console.log(`    andb server -mode mcp    ${DIM}Start MCP server (stdio)${RESET}`);
console.log();
console.log(`  ${BOLD}Docs${RESET}   ${CYAN}https://github.com/startvibecoding/AgentNativeDB${RESET}`);
console.log();
