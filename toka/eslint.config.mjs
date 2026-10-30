import parser from '@babel/eslint-parser';
import hooks from 'eslint-plugin-react-hooks';
import a11y from 'eslint-plugin-jsx-a11y';
export default [{
  ignores: ['dist/**', 'node_modules/**', 'src/app/**', '**/*.test.*', '**/test/**'],
}, {
  files: ['src/**/*.{ts,tsx,js,jsx}'],
  languageOptions: {
    parser,
    parserOptions: { requireConfigFile: false, babelOptions: { babelrc: false, configFile: false, presets: [['@babel/preset-typescript', { allExtensions: true, isTSX: true }]] }, ecmaFeatures: { jsx: true } },
  },
  plugins: { 'react-hooks': hooks, 'jsx-a11y': a11y },
  rules: { ...a11y.configs.recommended.rules, 'react-hooks/rules-of-hooks': 'error', 'react-hooks/exhaustive-deps': 'warn' },
}];
