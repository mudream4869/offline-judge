// Precompiles <bits/stdc++.h>, which takes most of the compile time.
//   node scripts/mkpch.mjs <clang bundle.js> <stdc++.h> <out.pch>
import fs from 'node:fs'
import { pathToFileURL } from 'node:url'
import { FLAGS, HEADER } from '../web/cppflags.mjs'

const [bundle, header, out] = process.argv.slice(2)
const { runClang } = await import(pathToFileURL(bundle).href)
const files = { inc: { bits: { 'stdc++.h': fs.readFileSync(header, 'utf8') } } }
const res = await runClang(['clang++', ...FLAGS, '-x', 'c++-header', HEADER, '-o', 'out.pch'], files)
fs.writeFileSync(out, res['out.pch'])
