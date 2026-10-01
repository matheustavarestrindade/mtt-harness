import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import ts from 'typescript';

const library = path.resolve('src/lib');
const levels = { atoms: 0, molecules: 1, organisms: 2 };
let failures = 0;

async function inspect(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      await inspect(filename);
      continue;
    }
    if (!/\.(ts|svelte)$/.test(filename)) continue;
    const layer = path.relative(library, filename).split(path.sep)[0];
    if (!(layer in levels)) {
      console.error(`Source outside a UI layer: ${filename}`);
      failures++;
      continue;
    }
    const contents = await readFile(filename, 'utf8');
    const scripts = filename.endsWith('.svelte')
      ? [...contents.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map((match) => match[1])
      : [contents];
    for (const script of scripts) {
      const source = ts.createSourceFile(
        filename,
        script,
        ts.ScriptTarget.Latest,
        true,
        ts.ScriptKind.TS,
      );
      function visit(node) {
        let specification;
        if (
          (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
          node.moduleSpecifier &&
          ts.isStringLiteral(node.moduleSpecifier)
        )
          specification = node.moduleSpecifier.text;
        if (
          ts.isCallExpression(node) &&
          node.expression.kind === ts.SyntaxKind.ImportKeyword &&
          node.arguments[0] &&
          ts.isStringLiteral(node.arguments[0])
        )
          specification = node.arguments[0].text;
        if (specification) {
          const target = specification.startsWith('$lib/')
            ? path.join(library, specification.slice(5))
            : specification.startsWith('.')
              ? path.resolve(path.dirname(filename), specification)
              : null;
          if (target) {
            const targetLayer = path.relative(library, target).split(path.sep)[0];
            if (!(targetLayer in levels) || levels[targetLayer] > levels[layer]) {
              console.error(
                `${path.relative(process.cwd(), filename)}: ${layer} must not import ${specification}`,
              );
              failures++;
            }
          }
        }
        ts.forEachChild(node, visit);
      }
      visit(source);
    }
  }
}

await inspect(library);
if (failures) process.exitCode = 1;
else console.log('UI layers: atoms <- molecules <- organisms. No upward imports.');
