// Generate reference results using the official pixelmatch v8.0.0 source:
// curl -fsSL https://raw.githubusercontent.com/mapbox/pixelmatch/v8.0.0/index.js -o /tmp/pixelmatch-v8.mjs
// node testdata/generate-v8.mjs /tmp/pixelmatch-v8.mjs > testdata/pixelmatch-v8.json
import fs from 'node:fs';
import {pathToFileURL} from 'node:url';
import {createHash} from 'node:crypto';

const source = process.argv[2];
const {default: match} = await import(pathToFileURL(source));
const cases = [];
const encode = bytes => Buffer.from(bytes).toString('base64');
function add(name, width, height, a, b, options = {}) {
    a = new Uint8Array(a);
    b = new Uint8Array(b);
    const output = new Uint8Array(a.length);
    const diff = match(a, b, output, width, height, options);
    if (match(a, b, null, width, height, options) !== diff) throw new Error(name);
    const storedOptions = {...options};
    if (options.ignoreMask) storedOptions.ignoreMask = encode(options.ignoreMask);
    cases.push({name, width, height, a: encode(a), b: encode(b), options: storedOptions, diff, output: encode(output)});
}

const gray = v => [v, v, v, 255];
for (const [name, a, b, options] of [
    ['near-black 13', gray(0), gray(13), {}],
    ['near-black 23', gray(0), gray(23), {}],
    ['near-black red', gray(0), [1, 0, 0, 255], {threshold: 0.05}],
    ['near-black green', gray(0), [0, 1, 0, 255], {threshold: 0.05}],
    ['near-black blue', gray(0), [0, 0, 1, 255], {threshold: 0.05}],
    ['visible dark step', gray(0), [32, 20, 20, 255], {threshold: 0.05}],
    ['blue separation', [41, 56, 157, 255], [41, 56, 0, 255], {}],
    ['dark blue separation', [39, 44, 92, 255], [39, 44, 14, 255], {}],
    ['cyan similarity', [0, 254, 252, 255], [94, 254, 252, 255], {}],
    ['cyan near similarity', [0, 254, 252, 255], [47, 254, 252, 255], {}],
    ['black-white threshold 1', gray(0), gray(255), {threshold: 1}],
    ['black-white above threshold 1', gray(0), gray(255), {threshold: 1.000001}],
    ['alpha checkerboard', [0, 0, 0, 128], gray(127), {}],
    ['alpha white', [0, 0, 0, 128], gray(127), {checkerboard: false}],
    ['transparent hidden red', [0, 0, 0, 0], [255, 0, 0, 0], {threshold: 0}],
    ['gray rounding', gray(49), gray(49), {alpha: 0.5}],
    ['gray fractional alpha', [30, 60, 90, 85], [30, 60, 90, 85], {alpha: 0.5}],
    ['ignore all', gray(0), gray(255), {ignoreMask: new Uint8Array([255])}],
    ['ignore all diff mask', gray(0), gray(255), {ignoreMask: new Uint8Array([1]), diffMask: true}]
]) add(name, 1, 1, a, b, options);

const blockA = new Uint8Array(400).fill(255);
const blockB = blockA.slice();
for (let y = 2; y < 5; y++) for (let x = 2; x < 5; x++) blockB.set(gray(0), (y * 10 + x) * 4);
for (const size of [1, 2, 3, 100, 2147483648, 9007199254740991, 2.9, 0, -5]) {
    add(`window ${size}`, 10, 10, blockA, blockB, {includeAA: true, windowSize: size});
}
add('identical window', 10, 10, blockA, blockA, {windowSize: 3});
add('rectangular clamped window', 10, 2, new Uint8Array(80).fill(255), new Uint8Array(80), {includeAA: true, windowSize: 32});

const rampA = [], rampB = [];
for (let y = 0; y < 3; y++) for (let x = 0; x < 3; x++) {
    rampA.push(...gray([0, 128, 255][x]));
    rampB.push(...gray([0, 192, 255][x]));
}
for (const includeAA of [false, true]) for (const diffMask of [false, true]) {
    add(`ramp aa=${includeAA} mask=${diffMask}`, 3, 3, rampA, rampB, {includeAA, diffMask, windowSize: 2, aaColor: [20, 40, 60]});
}

const alphaRampA = [], alphaRampB = [];
for (let y = 0; y < 5; y++) for (const alpha of [0, 85, 170, 255, 255]) {
    alphaRampA.push(255, 255, 255, alpha);
    alphaRampB.push(255, 255, 255, alpha === 85 ? 170 : alpha);
}
add('white alpha ramp', 5, 5, alphaRampA, alphaRampB);
add('white alpha ramp window', 5, 5, alphaRampA, alphaRampB, {windowSize: 2});

let seed = 8;
function random() {
    seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
    return seed >>> 24;
}
for (const opaque of [true, false]) {
    const w = 13, h = 7;
    const a = new Uint8Array(w * h * 4);
    for (let i = 0; i < a.length; i++) a[i] = i % 4 === 3 && opaque ? 255 : random();
    const b = a.slice();
    for (let i = 0; i < b.length; i += 4) {
        if (random() < 96) b[i] = random();
        if (random() < 96) b[i + 1] = random();
        if (!opaque && random() < 64) b[i + 3] = random();
    }
    for (const threshold of [0, 0.05, 0.1, 0.3, 1]) for (const includeAA of [false, true]) {
        add(`random opaque=${opaque} threshold=${threshold} aa=${includeAA}`, w, h, a, b, {threshold, includeAA});
    }
    add(`random colors opaque=${opaque}`, w, h, a, b, {alpha: 0.37, aaColor: [31, 73, 119], diffColor: [15, 25, 35], diffColorAlt: [235, 225, 215]});
    add(`random white opaque=${opaque}`, w, h, a, b, {checkerboard: false});
    add(`random window mask opaque=${opaque}`, w, h, a, b, {windowSize: 2.9, diffMask: true});
    const ignoreMask = Uint8Array.from({length: w * h}, (_, i) => i % 3 === 0 ? 255 : 0);
    add(`random ignore opaque=${opaque}`, w, h, a, b, {ignoreMask, windowSize: 3, diffMask: true});
}

console.log(JSON.stringify({version: '8.0.0', source: 'https://raw.githubusercontent.com/mapbox/pixelmatch/v8.0.0/index.js', sha256: createHash('sha256').update(fs.readFileSync(source)).digest('hex'), cases}, null, 2));
