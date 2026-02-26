import resolve from '@rollup/plugin-node-resolve';
import typescript from '@rollup/plugin-typescript';
import terser from '@rollup/plugin-terser';

export default [
  // IIFE bundle for <script> tag usage
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/clicknest.js',
      format: 'iife',
      name: 'clicknest',
      sourcemap: false,
    },
    plugins: [
      resolve(),
      typescript({ tsconfig: './tsconfig.json', declaration: false }),
      terser({
        compress: { passes: 2 },
        mangle: true,
      }),
    ],
  },
  // ESM bundle for npm import
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/clicknest.esm.js',
      format: 'es',
      sourcemap: false,
    },
    plugins: [
      resolve(),
      typescript({ tsconfig: './tsconfig.json' }),
      terser({
        compress: { passes: 2 },
        mangle: true,
      }),
    ],
  },
];
