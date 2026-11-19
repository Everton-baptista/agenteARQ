#!/usr/bin/env node
// Relative-link checker shared by the static-site and docs verifier images. No dependencies.
//
//   node agentarch-links.js html       every relative href/src/srcset/poster in *.html and every
//                                      url()/@import in CSS points to a file in the project
//   node agentarch-links.js markdown   every relative link and image in *.md resolves
//
// Runs in /workspace, the disposable copy of the project. External URLs (any scheme, or //host)
// are never fetched: verification has no network. Fragments and queries are ignored, so anchors
// are not checked. Nothing in the project configures this checker.
//
// Exit codes for the AgenteARQ runner: 0 every reference resolves, 1 broken reference or nothing
// to check (failed), 125 the checker could not decide (inconclusive).
'use strict';

const fs = require('fs');
const path = require('path');

const SKIP_DIRS = new Set(['.git', 'node_modules']);

// Project files below root, as sorted POSIX paths. Symlinked directories are not followed;
// a symlink to a regular file counts as that file.
function walk(root, accept) {
  const out = [];
  const visit = (rel) => {
    const entries = fs.readdirSync(path.join(root, rel), { withFileTypes: true });
    entries.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    for (const e of entries) {
      const r = rel ? `${rel}/${e.name}` : e.name;
      if (e.isDirectory()) {
        if (!SKIP_DIRS.has(e.name)) visit(r);
      } else if (accept(r)) {
        if (e.isFile()) out.push(r);
        else if (e.isSymbolicLink()) {
          try {
            if (fs.statSync(path.join(root, r)).isFile()) out.push(r);
          } catch { /* dangling symlink: not a file */ }
        }
      }
    }
  };
  visit('');
  return out;
}

// Runs a check and maps an unexpected error to "inconclusive" rather than a pass or a fail.
function run(name, fn) {
  Promise.resolve()
    .then(fn)
    .then((code) => { process.exitCode = code; })
    .catch((e) => {
      console.log(`${name}: could not decide (${e && e.message ? e.message : e}); result inconclusive`);
      process.exitCode = 125;
    });
}

// Replaces every match with spaces of the same length, keeping newlines, so offsets and line
// numbers stay valid after code, comments and script bodies are hidden.
const blank = (s) => s.replace(/[^\n]/g, ' ');
function mask(text, re, group) {
  return text.replace(re, (...m) => {
    if (group === undefined) return blank(m[0]);
    const whole = m[0];
    const inner = m[group];
    const at = whole.indexOf(inner, m[1] ? m[1].length : 0);
    return whole.slice(0, at) + blank(inner) + whole.slice(at + inner.length);
  });
}

function lineOf(text, offset) {
  let n = 1;
  for (let i = 0; i < offset && i < text.length; i++) if (text.charCodeAt(i) === 10) n++;
  return n;
}

function decodeEntities(s) {
  return s.replace(/&(#x[0-9a-f]+|#[0-9]+|amp|lt|gt|quot|apos|#39);/gi, (all, e) => {
    const k = e.toLowerCase();
    if (k === 'amp') return '&';
    if (k === 'lt') return '<';
    if (k === 'gt') return '>';
    if (k === 'quot') return '"';
    if (k === 'apos' || k === '#39') return "'";
    const code = k[1] === 'x' ? parseInt(k.slice(2), 16) : parseInt(k.slice(1), 10);
    try { return String.fromCodePoint(code); } catch { return all; }
  });
}

// --- extraction -------------------------------------------------------------------------------

function cssRefs(css, offset, out) {
  const text = mask(css, /\/\*[\s\S]*?\*\//g);
  for (const m of text.matchAll(/url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s"']*))\s*\)/gi)) {
    out.push({ target: m[1] ?? m[2] ?? m[3] ?? '', at: offset + m.index });
  }
  for (const m of text.matchAll(/@import\s+(?:"([^"]*)"|'([^']*)')/gi)) {
    out.push({ target: m[1] ?? m[2], at: offset + m.index });
  }
  return out;
}

const URL_ATTRS = new Set(['href', 'src', 'poster']);
const SRCSET_ATTRS = new Set(['srcset', 'imagesrcset']);

// References in HTML markup. Comments and script bodies are ignored; <style> bodies and style=""
// attributes are read as CSS. Returns { refs, base } where base is a <base href> value, if any.
function htmlRefs(source) {
  const refs = [];
  let base;
  let text = mask(source, /<!--[\s\S]*?-->/g);
  for (const m of text.matchAll(/(<style\b[^>]*>)([\s\S]*?)<\/style\s*>/gi)) {
    cssRefs(m[2], m.index + m[1].length, refs);
  }
  text = mask(text, /(<(script|style)\b(?:[^>"']|"[^"]*"|'[^']*')*>)([\s\S]*?)(?=<\/\2\s*>)/gi, 3);
  for (const t of text.matchAll(/<([a-zA-Z][a-zA-Z0-9-]*)((?:[^>"']|"[^"]*"|'[^']*')*)>/g)) {
    const tag = t[1].toLowerCase();
    const attrsAt = t.index + 1 + t[1].length;
    for (const a of t[2].matchAll(/([^\s"'=<>/]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g)) {
      const name = a[1].toLowerCase();
      const raw = a[2] ?? a[3] ?? a[4];
      if (raw === undefined) continue;
      const value = decodeEntities(raw);
      const at = attrsAt + a.index;
      if (tag === 'base' && name === 'href') base = value.trim();
      else if (URL_ATTRS.has(name)) refs.push({ target: value, at });
      else if (SRCSET_ATTRS.has(name)) {
        for (const candidate of value.split(',')) {
          const url = candidate.trim().split(/\s+/)[0];
          if (url) refs.push({ target: url, at });
        }
      } else if (name === 'style') cssRefs(value, at, refs);
    }
  }
  return { refs, base };
}

// Markdown links, images and reference definitions, plus HTML embedded in the Markdown.
// Fenced code blocks, inline code and HTML comments are ignored.
function markdownRefs(source) {
  const lines = source.split('\n');
  let fence = null;
  for (let i = 0; i < lines.length; i++) {
    const m = /^ {0,3}(`{3,}|~{3,})/.exec(lines[i]);
    if (fence) {
      if (m && m[1][0] === fence[0] && m[1].length >= fence.length && /^\s*$/.test(lines[i].slice(m[0].length))) fence = null;
      lines[i] = blank(lines[i]);
    } else if (m && !(m[1][0] === '`' && lines[i].slice(m[0].length).includes('`'))) {
      fence = m[1];
      lines[i] = blank(lines[i]);
    }
  }
  let text = lines.join('\n');
  text = mask(text, /<!--[\s\S]*?-->/g);
  text = mask(text, /(`+)(?!`)(?:(?!\n[ \t]*\n)[\s\S])*?(?<!`)\1(?!`)/g);
  const refs = [];
  const inline = /!?\[((?:\\.|[^[\]\\]|\[(?:\\.|[^[\]\\])*\])*)\]\(\s*(<[^<>\n]*>|(?:\\.|[^\s()\\]|\((?:\\.|[^\s()\\])*\))*)(?:\s+(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\((?:\\.|[^()\\])*\)))?\s*\)/g;
  for (const m of text.matchAll(inline)) refs.push({ target: unescapeMd(m[2]), at: m.index });
  for (const m of text.matchAll(/^ {0,3}\[((?:\\.|[^\]\\])+)\]:[ \t]*\n?[ \t]*(<[^<>\n]*>|\S+)/gm)) {
    if (m[1].startsWith('^')) continue; // footnote definition, not a link
    refs.push({ target: unescapeMd(m[2]), at: m.index });
  }
  for (const r of htmlRefs(text).refs) refs.push(r);
  return refs;
}

function unescapeMd(dest) {
  let d = dest;
  if (d.startsWith('<') && d.endsWith('>')) d = d.slice(1, -1);
  return decodeEntities(d.replace(/\\([!-/:-@[-`{-~])/g, '$1'));
}

// --- resolution -------------------------------------------------------------------------------

// A reference that is not a project path: empty, fragment- or query-only (the same document), any
// URL scheme (http:, mailto:, data:, javascript: ...), protocol-relative, or a template placeholder.
function isExternal(target) {
  const t = target.trim();
  return t === '' || t.startsWith('#') || t.startsWith('?') || t.startsWith('//') || /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(t) ||
    /\{\{|\{%|\$\{|<%/.test(t);
}

// Resolves target against fromDir (POSIX, relative to the root). Root-relative paths ("/a/b")
// resolve against the project root. Returns null when the path leaves the project.
function resolveTarget(target, fromDir) {
  let p = target.trim().replace(/[?#].*$/s, '');
  try { p = decodeURIComponent(p); } catch { /* keep the raw path */ }
  const joined = p.startsWith('/') ? path.posix.normalize(p).replace(/^\/+/, '') : path.posix.normalize(path.posix.join(fromDir, p));
  if (joined === '..' || joined.startsWith('../')) return null;
  return { path: joined === '.' ? '' : joined.replace(/\/$/, ''), dir: /\/$/.test(p) || p === '' };
}

function exists(root, rel, { html, wantDir }) {
  let st;
  try { st = fs.statSync(path.join(root, rel || '.')); } catch { return false; }
  if (st.isFile()) return !wantDir;
  if (!st.isDirectory()) return false;
  // A web server answers a directory request with its index page; Markdown viewers list the
  // directory (or render its README), so a directory is enough there.
  if (!html) return true;
  try { return fs.statSync(path.join(root, rel, 'index.html')).isFile(); } catch { return false; }
}

// --- main -------------------------------------------------------------------------------------

function check(mode, root) {
  const html = mode === 'html';
  const primary = html ? walk(root, (f) => /\.html?$/i.test(f)) : walk(root, (f) => /\.(md|markdown)$/i.test(f));
  if (primary.length === 0) {
    console.log(html
      ? 'links: no .html files found; a static site needs at least one page, so this cannot pass'
      : 'links: no Markdown (.md) files found; a documentation project needs at least one, so this cannot pass');
    return 1;
  }
  const files = html ? primary.concat(walk(root, (f) => /\.css$/i.test(f))) : primary;
  let total = 0;
  const broken = [];
  for (const file of files) {
    const source = fs.readFileSync(path.join(root, file), 'utf8');
    let refs;
    let fromDir = path.posix.dirname(file);
    if (file.toLowerCase().endsWith('.css')) refs = cssRefs(source, 0, []);
    else if (html) {
      const found = htmlRefs(source);
      refs = found.refs;
      if (found.base !== undefined && found.base !== '') {
        if (isExternal(found.base)) {
          console.log(`links: ${file}: <base href="${found.base}"> points outside the project; its relative links are not checked`);
          continue;
        }
        const b = resolveTarget(found.base.endsWith('/') ? found.base : `${found.base}/`, fromDir);
        if (b) fromDir = b.path || '.';
      }
    } else refs = markdownRefs(source);
    for (const r of refs) {
      if (isExternal(r.target)) continue;
      total++;
      const resolved = resolveTarget(r.target, fromDir);
      const line = lineOf(source, r.at);
      if (resolved === null) broken.push(`${file}:${line}: "${r.target}" points outside the project`);
      else if (!exists(root, resolved.path, { html, wantDir: resolved.dir })) {
        const what = resolved.dir && html ? `${resolved.path || '.'}/index.html` : resolved.path || '.';
        broken.push(`${file}:${line}: "${r.target}" -> ${what} does not exist`);
      }
    }
  }
  for (const b of broken) console.log(`links: ${b}`);
  const scope = `${total} relative reference${total === 1 ? '' : 's'} in ${files.length} file${files.length === 1 ? '' : 's'}`;
  if (broken.length) {
    console.log(`links: ${broken.length} broken of ${scope}`);
    return 1;
  }
  console.log(total
    ? `links: all ${scope} resolve (external URLs and anchors are not checked)`
    : `links: no relative references in ${files.length} file${files.length === 1 ? '' : 's'} (external URLs are not checked)`);
  return 0;
}

module.exports = { walk, run, htmlRefs, markdownRefs, isExternal, resolveTarget, check };

if (require.main === module) {
  const mode = process.argv[2];
  if (mode !== 'html' && mode !== 'markdown') {
    console.log('links: usage: agentarch-links.js html|markdown');
    process.exitCode = 125;
  } else run('links', () => check(mode, '.'));
}
