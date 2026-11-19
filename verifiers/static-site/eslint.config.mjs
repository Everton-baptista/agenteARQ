// static-site profile: the only ESLint configuration the `lint` check uses. Any eslint.config.*
// in the project is ignored. Recommended rules, browser globals, latest syntax.
//
// *.mjs is an ES module. *.js is a classic script unless agentarch-js-lint finds module syntax in
// it or a page loads it with <script type="module">; it then adds that file to the module list.
import js from '@eslint/js';
import globals from 'globals';

export default [
  // Vendored, minified libraries are not the project's source.
  { ignores: ['**/*.min.js', '**/*.min.mjs'] },
  js.configs.recommended,
  {
    files: ['**/*.js', '**/*.mjs'],
    languageOptions: { ecmaVersion: 'latest', globals: { ...globals.browser } },
    linterOptions: { reportUnusedDisableDirectives: 'off' },
  },
  {
    // A classic script's top-level functions are globals called from other scripts or from
    // onclick="" attributes, so only local variables are reported as unused.
    files: ['**/*.js'],
    languageOptions: { sourceType: 'script' },
    rules: { 'no-unused-vars': ['error', { vars: 'local' }] },
  },
  { files: ['**/*.mjs'], languageOptions: { sourceType: 'module' } },
];
