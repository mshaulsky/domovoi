// Subsets the Material Design Icons webfont to the code points given on the
// command line. Usage: node tools/subset-icons.mjs <in.ttf> <out.ttf> <HEX>...
// Requires the subset-font package (npm install subset-font).
import { readFileSync, writeFileSync } from "node:fs";
import subsetFont from "subset-font";

const [, , src, dst, ...codepoints] = process.argv;
if (!src || !dst || codepoints.length === 0) {
  console.error("usage: subset-icons.mjs <in.ttf> <out.ttf> <HEX>...");
  process.exit(2);
}
const text = codepoints.map((cp) => String.fromCodePoint(parseInt(cp, 16))).join("");
const out = await subsetFont(readFileSync(src), text, { targetFormat: "truetype" });
writeFileSync(dst, out);
console.log(`subset: ${codepoints.length} glyphs, ${out.length} bytes → ${dst}`);
