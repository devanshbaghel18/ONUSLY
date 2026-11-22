const TAU = 2 * Math.PI;
const steel = [134, 145, 152];
const graphite = [47, 53, 58];
const orange = [255, 105, 49];

function ring(inner, outer, bottom, top, material, segments = 64) {
  const at = (r, n, z) => [r * Math.cos(n * TAU / segments), r * Math.sin(n * TAU / segments), z];
  return Array.from({ length: segments }, (_, n) => {
    const a = at(outer, n, bottom), b = at(outer, n + 1, bottom);
    const c = at(outer, n + 1, top), d = at(outer, n, top);
    const e = at(inner, n, top), f = at(inner, n + 1, top);
    const g = at(inner, n + 1, bottom), h = at(inner, n, bottom);
    return [[a,b,c,d], [d,c,f,e], [e,f,g,h], [h,g,b,a]].map(vertices => ({ vertices, material }));
  }).flat();
}

function box(x, y, z, width, height, depth, material) {
  const vertices = [[0,0,0],[1,0,0],[1,1,0],[0,1,0],[0,0,1],[1,0,1],[1,1,1],[0,1,1]]
    .map(([u,v,w]) => [x + u * width, y + v * height, z + w * depth]);
  return [[3,2,1,0],[4,5,6,7],[0,1,5,4],[1,2,6,5],[2,3,7,6],[3,0,4,7]]
    .map(indices => ({ vertices: indices.map(i => vertices[i]), material }));
}

const housing = [
  ...ring(.05, 1.44, -.32, -.23, steel),
  ...ring(.07, 1.49, -.23, .03, graphite),
  ...ring(1.23, 1.49, .03, .17, steel),
  ...ring(1.17, 1.3, .13, .21, graphite),
  ...ring(1.145, 1.177, .21, .233, orange),
  ...ring(.05, 1.145, -.04, .035, [24, 33, 40]),
  ...ring(.08, .32, .04, .17, steel, 32),
  ...ring(.02, .15, .17, .18, graphite, 24),
  ...Array.from({ length: 16 }, (_, i) => {
    const angle = i * TAU / 16;
    return box(-.032, 1.37, .174, .064, .047, .021, graphite)
      .map(face => ({ ...face, vertices: face.vertices.map(([x,y,z]) => [
        x * Math.cos(angle) - y * Math.sin(angle), x * Math.sin(angle) + y * Math.cos(angle), z,
      ]) }));
  }).flat(),
];
// The aperture segments remain inside the housing as the bar releases.
function aperture(release) {
  return Array.from({ length: 8 }, (_, index) => {
    const angle = index * TAU / 8 + release * .23;
    const inner = .32 + release * .45;
    const polar = (radius, offset, z) => [radius * Math.cos(angle + offset), radius * Math.sin(angle + offset), z];
    return {
      material: index % 2 ? [84, 98, 107] : [111, 124, 131],
      vertices: [
        polar(inner, .55, .082),
        polar(1.10, -.12, .073),
        polar(1.10, .54, .087),
        polar(.75 + release * .20, .89, .097),
        polar(inner, 1.08, .092),
      ],
    };
  });
}

const hingeX = -1.19;
const bar = box(hingeX, -.47, .27, 2.38, .38, .16, steel);
const slider = box(-.17, -.50, .43, .58, .44, .14, orange);

export function paintLock(ctx, width, height, yaw, pitch, release, review) {
  const cy = Math.cos(yaw), sy = Math.sin(yaw);
  const cp = Math.cos(pitch), sp = Math.sin(pitch);
  const scale = Math.min(width, height) / 3.55;
  function rotate([x,y,z]) {
    const horizontal = x * cy - z * sy;
    const depth = x * sy + z * cy;
    return [horizontal, y * cp - depth * sp, y * sp + depth * cp];
  }

  const hingeAngle = release * 1.12;
  const mechanism = [...bar, ...slider].map((face, index) => ({
    ...face,
    vertices: face.vertices.map(([x,y,z]) => {
      const arm = x - hingeX + (index >= bar.length ? .36 * review - .32 * release : 0);
      return [hingeX + arm * Math.cos(hingeAngle), y, z + arm * Math.sin(hingeAngle)];
    }),
  }));

  ctx.clearRect(0, 0, width, height);
  const faces = [...housing, ...aperture(release), ...mechanism].map(face => {
    const vertices = face.vertices.map(rotate);
    return { vertices, material: face.material, depth: vertices.reduce((sum, v) => sum + v[2], 0) / vertices.length };
  }).sort((a,b) => a.depth - b.depth);

  for (const { vertices, material } of faces) {
    const [a,b,c] = vertices;
    const ux = b[0]-a[0], uy = b[1]-a[1], uz = b[2]-a[2];
    const vx = c[0]-a[0], vy = c[1]-a[1], vz = c[2]-a[2];
    const nx = uy*vz - uz*vy, ny = uz*vx - ux*vz, nz = ux*vy - uy*vx;
    if (nz <= 0) continue;
    const intensity = .50 + .66 * Math.max(0, (-.45*nx + .38*ny + .80*nz) / Math.hypot(nx,ny,nz));
    const tint = material.map(channel => Math.min(255, Math.round(channel * intensity)));
    ctx.fillStyle = ctx.strokeStyle = 'rgb(' + tint.join(',') + ')';
    ctx.lineWidth = .7;
    ctx.beginPath();
    for (const [x,y,z] of vertices) {
      const zoom = 7 / (7 - z);
      ctx.lineTo(width * .50 + x * scale * zoom, height * .51 - y * scale * zoom);
    }
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
  }
}
