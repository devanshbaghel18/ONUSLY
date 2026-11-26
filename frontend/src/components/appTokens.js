// Small enamel decals mapped onto the preview model's three metal inserts.
let decals;
function makeDecal(index) {
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = 128;
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = ["#5461da", "#c54079", "#183146"][index];
  ctx.fillRect(0, 0, 128, 128);
  ctx.strokeStyle = ctx.fillStyle = "#fffaf1";
  ctx.lineWidth = 7;
  ctx.lineCap = "round";
  if (index === 0) {
    ctx.beginPath();
    ctx.moveTo(24,82); ctx.quadraticCurveTo(24,58,38,40);
    ctx.quadraticCurveTo(64,30,90,40); ctx.quadraticCurveTo(104,57,104,82);
    ctx.lineTo(84,92); ctx.lineTo(77,82); ctx.quadraticCurveTo(64,87,51,82);
    ctx.lineTo(44,92); ctx.closePath(); ctx.fill();
    ctx.fillStyle = "#5461da";
    for (const x of [48,80]) { ctx.beginPath(); ctx.ellipse(x,66,6,8,0,0,Math.PI*2); ctx.fill(); }
  } else if (index === 1) {
    ctx.beginPath(); ctx.roundRect(29,29,70,70,19); ctx.stroke();
    ctx.beginPath(); ctx.arc(64,64,17,0,Math.PI*2); ctx.stroke();
    ctx.beginPath(); ctx.arc(85,44,5,0,Math.PI*2); ctx.fill();
  } else {
    ctx.beginPath(); ctx.moveTo(22,72); ctx.lineTo(54,86); ctx.lineTo(84,51); ctx.stroke();
    for (const [x,y,r] of [[54,86,10],[85,47,18]]) {
      ctx.beginPath(); ctx.arc(x,y,r,0,Math.PI*2); ctx.stroke();
    }
  }
  return canvas;
}

export function drawDecal(ctx, index, corners) {
  decals ??= [0,1,2].map(makeDecal);
  const [bottomLeft,,topRight,topLeft] = corners;
  ctx.save();
  ctx.clip();
  ctx.transform((topRight[0]-topLeft[0])/128,(topRight[1]-topLeft[1])/128,
    (bottomLeft[0]-topLeft[0])/128,(bottomLeft[1]-topLeft[1])/128,topLeft[0],topLeft[1]);
  ctx.drawImage(decals[index],0,0);
  ctx.restore();
}
