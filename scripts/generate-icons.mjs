// Generate icons from SVG source for all platforms
import { writeFileSync, readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import sharp from 'sharp';
import pngToIco from 'png-to-ico';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = resolve(__dirname, '..');

const svgPath = resolve(root, 'build', 'icon.svg');
const appiconPath = resolve(root, 'build', 'appicon.png');
const trayiconPath = resolve(root, 'build', 'trayicon.png');
const icoPath = resolve(root, 'build', 'windows', 'icon.ico');

const svg = readFileSync(svgPath);

console.log('Generating appicon.png (512x512)...');
await sharp(svg).resize(512, 512).png().toFile(appiconPath);

console.log('Generating trayicon.png (32x32)...');
await sharp(svg).resize(32, 32).png().toFile(trayiconPath);

console.log('Generating icon.ico...');
const icoBuf = await pngToIco(appiconPath);
writeFileSync(icoPath, icoBuf);

console.log('All icons generated successfully!');
