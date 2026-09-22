package editor

// pageHTML is the whole editor: one page, no framework. {{BASE}} is the
// path prefix the page was served under (e.g. "/ha-widget"), so its
// fetches work through a prefix-keeping reverse proxy too.
const pageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Floorplan editor</title>
<style>
:root{--bg:#1b1b23;--panel:#20202a;--border:#30303d;--text:#c9c9d6;--muted:#7f7f95;--hi:#f2f2f8;--accent:#a9c2f7;--warn:#f28b82;--ok:#8fd3a0;
  --color-widget-background:#20202a;--color-widget-content-border:#30303d;--color-text-base:#c9c9d6;--color-text-subdue:#7f7f95;--color-text-highlight:#f2f2f8;--color-primary:#a9c2f7;--color-negative:#f28b82;--border-radius:5px}
@media (prefers-color-scheme:light){:root{--bg:#f4f4f7;--panel:#fff;--border:#d9d9e2;--text:#2a2a36;--muted:#71718a;--hi:#111;--accent:#3b62c4;
  --color-widget-background:#fff;--color-widget-content-border:#d9d9e2;--color-text-base:#2a2a36;--color-text-subdue:#71718a;--color-text-highlight:#111;--color-primary:#3b62c4}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:13px/1.5 "JetBrains Mono",ui-monospace,Menlo,monospace}
header{display:flex;align-items:center;gap:12px;padding:12px 18px;border-bottom:1px solid var(--border);position:sticky;top:0;background:var(--bg);z-index:5}
header h1{font-size:15px;margin:0 auto 0 0;color:var(--hi)}
button{background:var(--panel);color:var(--text);border:1px solid var(--border);border-radius:6px;padding:6px 12px;font:inherit;cursor:pointer}
button:hover{border-color:var(--accent);color:var(--hi)}
button.primary{background:var(--accent);color:#111;border-color:var(--accent)}
button.danger:hover{border-color:var(--warn);color:var(--warn)}
input,select{background:var(--bg);color:var(--text);border:1px solid var(--border);border-radius:5px;padding:4px 7px;font:inherit}
input[type=number]{width:64px}
main{display:grid;grid-template-columns:minmax(360px,1fr) minmax(360px,1fr);gap:18px;padding:18px;align-items:start}
section{background:var(--panel);border:1px solid var(--border);border-radius:8px;padding:14px}
section h2{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--muted);margin:0 0 10px}
.row{display:flex;flex-wrap:wrap;gap:8px 12px;align-items:center;margin-bottom:10px}
.row label{display:inline-flex;align-items:center;gap:6px;color:var(--muted)}
#mapWrap{position:relative;max-width:100%}
#map{display:grid;gap:3px;background:var(--border);padding:3px;border-radius:6px;user-select:none;aspect-ratio:var(--aspect,1.15);max-width:100%}
#map .cell .grip{cursor:move;padding:1px 6px;margin:-3px -5px;border-radius:3px;background:rgba(0,0,0,.18)}
#map .cell .grip:hover{background:var(--accent);color:#111}
.h{position:absolute;z-index:3;background:var(--accent);opacity:.55;border-radius:3px}
.h:hover,.h.active{opacity:1}
.h.t,.h.b{height:6px;cursor:ns-resize}
.h.l,.h.r{width:6px;cursor:ew-resize}
body.dragging{cursor:grabbing}
body.dragging *{cursor:inherit!important}
#map .cell{background:var(--bg);border-radius:3px;min-height:18px;position:relative;cursor:crosshair;display:flex;align-items:flex-start;padding:3px 5px;font-size:11px;color:var(--hi);overflow:hidden;white-space:nowrap}
#map .cell.room{background:var(--rc)}
#map .cell.sel{outline:2px solid var(--accent);outline-offset:-2px}
#rooms{display:flex;flex-direction:column;gap:6px;margin-bottom:12px}
.room{display:grid;grid-template-columns:14px 1fr 1fr 1.2fr auto;gap:8px;align-items:center;padding:6px 8px;border:1px solid var(--border);border-radius:6px;cursor:pointer}
.room.sel{border-color:var(--accent)}
.room .sw{width:14px;height:14px;border-radius:3px}
.room input,.room select{width:100%;min-width:0}
#inner{display:grid;gap:4px;background:var(--border);padding:4px;border-radius:6px;aspect-ratio:1.2;max-width:420px;max-height:520px;margin-bottom:12px}
#inner .icell{background:var(--bg);border-radius:4px;display:flex;flex-wrap:wrap;gap:4px;align-items:center;justify-content:center;min-height:44px;padding:4px}
#inner .icell.over{outline:2px dashed var(--accent);outline-offset:-2px}
#inner .icell.center{background:color-mix(in srgb,var(--accent) 8%,var(--bg))}
.ent{display:inline-flex;align-items:center;gap:6px;padding:4px 8px;border:1px solid var(--border);border-radius:14px;background:var(--panel);cursor:grab;font-size:12px;max-width:100%}
.ent svg{width:16px;height:16px;fill:var(--text);flex:none}
.ent .k{color:var(--muted);font-size:10px}
.ent.hidden{opacity:.45;text-decoration:line-through}
.ent button{padding:0 4px;border:0;background:none;color:var(--muted);font-size:12px}
#list{display:flex;flex-wrap:wrap;gap:6px;min-height:40px;padding:6px;border:1px dashed var(--border);border-radius:6px}
#list.over{border-color:var(--accent)}
#msg{padding:0 18px;color:var(--warn);min-height:20px}
#msg.ok{color:var(--ok)}
#preview{grid-column:1/-1}
#preview .frame{background:var(--color-widget-background);border:1px solid var(--border);border-radius:8px;padding:14px;max-width:900px}
.hint{color:var(--muted);font-size:12px;margin:0 0 10px}
</style>
</head>
<body>
<header>
  <h1>Floorplan editor</h1>
  <button id="btnPreview">Preview</button>
  <button id="btnReset" class="danger" title="Discard the saved layout and start again from config">Reset to config</button>
  <button id="btnSave" class="primary">Save</button>
</header>
<div id="msg"></div>
<main>
  <section>
    <h2>Map</h2>
    <div class="row">
      <label>columns <input type="number" id="cols" min="1" max="24"></label>
      <label>rows <input type="number" id="rows" min="1" max="24"></label>
      <label>aspect <input type="text" id="aspect" size="6" placeholder="1.15"></label>
      <label>max width px <input type="number" id="maxw" min="0" step="10"></label>
    </div>
    <p class="hint">Select a room, then paint cells (click/drag; ⌥/Alt-click removes), drag the blue handles on its edges to resize, or drag it by its name to move it. Rooms stay solid rectangles; a change that would break another room is refused.</p>
    <div id="rooms"></div>
    <div class="row"><button id="btnAddRoom">+ room</button></div>
    <div id="mapWrap"><div id="map"></div></div>
  </section>
  <section>
    <h2>Room <span id="roomTitle"></span></h2>
    <div class="row">
      <label><input type="checkbox" id="igridAuto"> grid follows the map</label>
      <label>inner grid columns <input type="number" id="icols" min="1" max="12"></label>
      <label>rows <input type="number" id="irows" min="1" max="12"></label>
    </div>
    <p class="hint">Drag entities from the list onto a cell. Edge cells hug the wall and beam toward the centre; the centre cell casts no beam. Drag back to the list to unplace, ⊘ to hide.</p>
    <div id="inner"></div>
    <div class="row"><label><input type="checkbox" id="showAll"> show all entities (sensors, buttons, …)</label></div>
    <div id="list"></div>
  </section>
  <section id="preview" hidden>
    <h2>Preview</h2>
    <div class="frame" id="previewFrame"></div>
  </section>
</main>
<script>
(function(){
const BASE="{{BASE}}";
const $=s=>document.querySelector(s);
const palette=["#4d6fb3","#8a5cb8","#3f9b7a","#b8783f","#b04f5f","#4f9bb0","#7a8a3f","#9b5c8f","#5c7a9b","#a08a3f"];
let state={layout:null,seed:null,areas:[],sel:0,painting:false};
const msg=(t,ok)=>{const m=$("#msg");m.textContent=t||"";m.className=ok?"ok":"";};

async function load(){
  const r=await fetch(BASE+"/edit/data.json",{cache:"no-store"});
  if(!r.ok){msg("Failed to load: "+await r.text());return;}
  const d=await r.json();
  state.layout=normalize(d.layout);state.seed=d.seed;state.areas=d.areas;
  renderAll();
}
// The server omits empty lists/maps (omitempty); give every room the
// fields the UI touches so a freshly saved or seeded layout is editable.
function normalize(L){
  L=L||{};L.rooms=L.rooms||[];L.columns=L.columns||1;L.rows=L.rows||1;
  L.rooms.forEach(rm=>{rm.entities=rm.entities||{};rm.hidden=rm.hidden||[];rm.cells=rm.cells||[];rm.name=rm.name||"";
    if(!rm.grid||(!rm.grid.rows&&!rm.grid.columns))rm.grid={auto:true};syncGrid(rm);});
  return L;
}
// An automatic inner grid mirrors the room's footprint on the map (a 2x4
// room → 2x4 grid). Editing the inner grid by hand pins it (auto=false).
function syncGrid(rm){
  if(!rm.grid.auto||!rm.cells.length)return;
  const [r0,c0,r1,c1]=bounds(rm.cells);const rows=r1-r0+1,cols=c1-c0+1;
  if(rm.grid.rows!==rows||rm.grid.columns!==cols){rm.grid.rows=rows;rm.grid.columns=cols;clampInner(rm);}
}
// The inner grid box takes the room's real proportions: cell aspect from
// the map's aspect ratio, times the footprint.
function innerAspect(rm){
  const L=state.layout;if(!rm.cells.length)return 1.2;
  const [r0,c0,r1,c1]=bounds(rm.cells);const a=parseAspect(L.aspect_ratio)||(L.columns/L.rows);
  const cellAspect=a*L.rows/L.columns;return Math.max(.3,Math.min(4,cellAspect*(c1-c0+1)/(r1-r0+1)));
}
function parseAspect(v){if(!v)return 0;const m=String(v).trim().match(/^(\d+(?:\.\d+)?)(?:\s*\/\s*(\d+(?:\.\d+)?))?$/);if(!m)return 0;return m[2]?+m[1]/+m[2]:+m[1];}
function color(i){return palette[i%palette.length];}
function room(){return state.layout.rooms[state.sel];}
function areaOf(rm){return state.areas.find(a=>a.name===rm.area)||{entities:[]};}

function renderAll(){renderControls();renderRooms();renderMap();renderRoom();}
function renderControls(){
  const L=state.layout;
  $("#cols").value=L.columns;$("#rows").value=L.rows;$("#aspect").value=L.aspect_ratio||"";$("#maxw").value=L.max_width||0;
  $("#map").style.setProperty("--aspect",L.aspect_ratio||(L.columns+"/"+L.rows));
}
function renderRooms(){
  const box=$("#rooms");box.innerHTML="";
  state.layout.rooms.forEach((rm,i)=>{
    const el=document.createElement("div");el.className="room"+(i===state.sel?" sel":"");
    el.innerHTML='<span class="sw" style="background:'+color(i)+'"></span>'+
      '<input class="key" value="'+esc(rm.key)+'" title="key (used in CSS grid)">'+
      '<input class="name" value="'+esc(rm.name||"")+'" placeholder="display name (optional)">'+
      '<select class="area"><option value="">— area —</option>'+state.areas.map(a=>'<option'+(a.name===rm.area?" selected":"")+'>'+esc(a.name)+'</option>').join("")+'</select>'+
      '<button class="danger del" title="delete room">✕</button>';
    el.addEventListener("click",e=>{if(e.target.tagName==="BUTTON")return;state.sel=i;renderRooms();renderMap();renderRoom();});
    el.querySelector(".key").addEventListener("change",e=>{rm.key=e.target.value.trim().toLowerCase().replace(/[^a-z0-9_-]/g,"-");e.target.value=rm.key;renderMap();});
    el.querySelector(".name").addEventListener("change",e=>{rm.name=e.target.value.trim();renderMap();renderRoom();});
    el.querySelector(".area").addEventListener("change",e=>{rm.area=e.target.value;rm.entities={};rm.hidden=[];renderMap();renderRoom();});
    el.querySelector(".del").addEventListener("click",()=>{state.layout.rooms.splice(i,1);state.sel=Math.max(0,Math.min(state.sel,state.layout.rooms.length-1));renderAll();});
    box.appendChild(el);
  });
}
function owner(r,c){return state.layout.rooms.findIndex(rm=>rm.cells.some(x=>x[0]===r&&x[1]===c));}
function renderMap(){
  const L=state.layout,map=$("#map");map.innerHTML="";
  L.rooms.forEach(syncGrid);
  map.style.gridTemplateColumns="repeat("+L.columns+",1fr)";map.style.gridTemplateRows="repeat("+L.rows+",1fr)";
  for(let r=0;r<L.rows;r++)for(let c=0;c<L.columns;c++){
    const cell=document.createElement("div");cell.className="cell";const o=owner(r,c);
    if(o>=0){cell.classList.add("room");cell.style.setProperty("--rc",color(o)+"55");if(o===state.sel)cell.classList.add("sel");
      const rm=L.rooms[o];const first=rm.cells[0];
      if(first[0]===r&&first[1]===c){const g=document.createElement("span");g.className="grip";g.textContent=rm.name||rm.area||rm.key;g.title="drag to move the room";
        g.addEventListener("mousedown",e=>{e.preventDefault();e.stopPropagation();state.sel=o;renderRooms();renderMap();renderRoom();startMove(e);});cell.appendChild(g);}}
    cell.addEventListener("mousedown",e=>{e.preventDefault();state.painting=true;paint(r,c,e.altKey);});
    cell.addEventListener("mouseenter",()=>{if(state.painting)paint(r,c,false);});
    map.appendChild(cell);
  }
  renderHandles();
}
document.addEventListener("mouseup",()=>state.painting=false);

// --- resize handles + move: whole-cell steps, rectangle preserved ---
function cellEl(r,c){return $("#map").children[r*state.layout.columns+c];}
function renderHandles(){
  document.querySelectorAll("#mapWrap .h").forEach(h=>h.remove());
  const rm=room();if(!rm||!rm.cells.length)return;
  const [r0,c0,r1,c1]=bounds(rm.cells);const wrap=$("#mapWrap").getBoundingClientRect();
  const a=cellEl(r0,c0).getBoundingClientRect(),b=cellEl(r1,c1).getBoundingClientRect();
  const L=a.left-wrap.left,T=a.top-wrap.top,R=b.right-wrap.left,B=b.bottom-wrap.top;
  const mk=(cls,st)=>{const h=document.createElement("div");h.className="h "+cls;Object.assign(h.style,st);h.addEventListener("mousedown",e=>{e.preventDefault();e.stopPropagation();startResize(e,cls,h);});$("#mapWrap").appendChild(h);};
  mk("t",{left:L+8+"px",width:(R-L-16)+"px",top:(T-3)+"px"});
  mk("b",{left:L+8+"px",width:(R-L-16)+"px",top:(B-3)+"px"});
  mk("l",{top:T+8+"px",height:(B-T-16)+"px",left:(L-3)+"px"});
  mk("r",{top:T+8+"px",height:(B-T-16)+"px",left:(R-3)+"px"});
}
function cellStep(){const a=cellEl(0,0).getBoundingClientRect();const w=state.layout.columns>1?cellEl(0,1).getBoundingClientRect().left-a.left:a.width+3;const h=state.layout.rows>1?cellEl(1,0).getBoundingClientRect().top-a.top:a.height+3;return [w,h];}
function dragSession(e,onDelta){
  const [sw,sh]=cellStep();const x0=e.clientX,y0=e.clientY;document.body.classList.add("dragging");
  let last=[0,0];
  const move=ev=>{const d=[Math.round((ev.clientX-x0)/sw),Math.round((ev.clientY-y0)/sh)];if(d[0]===last[0]&&d[1]===last[1])return;last=d;onDelta(d[0],d[1]);};
  const up=()=>{document.removeEventListener("mousemove",move);document.removeEventListener("mouseup",up);document.body.classList.remove("dragging");renderMap();};
  document.addEventListener("mousemove",move);document.addEventListener("mouseup",up);
}
function startResize(e,side,h){
  const rm=room();const start=bounds(rm.cells);h.classList.add("active");
  dragSession(e,(dc,dr)=>{
    let [r0,c0,r1,c1]=start;
    if(side==="t")r0=Math.min(r0+dr,r1);if(side==="b")r1=Math.max(r1+dr,r0);
    if(side==="l")c0=Math.min(c0+dc,c1);if(side==="r")c1=Math.max(c1+dc,c0);
    applyRect(state.sel,[r0,c0,r1,c1]);
  });
}
function startMove(e){
  const rm=room();const start=bounds(rm.cells);const L=state.layout;
  dragSession(e,(dc,dr)=>{
    const h=start[2]-start[0],w=start[3]-start[1];
    const r0=Math.max(0,Math.min(L.rows-1-h,start[0]+dr)),c0=Math.max(0,Math.min(L.columns-1-w,start[1]+dc));
    applyRect(state.sel,[r0,c0,r0+h,c0+w]);
  });
}
// applyRect gives room idx the rectangle and takes those cells from the
// others. A neighbour that would be left non-rectangular is trimmed to its
// largest remaining rectangle instead (the cut-off cells become free), so
// dragging a wall "pushes" the neighbour the way a real splitter would.
// Refused only if a neighbour would vanish entirely.
function applyRect(idx,[r0,c0,r1,c1]){
  const L=state.layout;
  if(r0<0||c0<0||r1>=L.rows||c1>=L.columns)return false;
  const rect=[];for(let r=r0;r<=r1;r++)for(let c=c0;c<=c1;c++)rect.push([r,c]);
  const inRect=([r,c])=>r>=r0&&r<=r1&&c>=c0&&c<=c1;
  const others=[];
  for(let i=0;i<L.rooms.length;i++){
    if(i===idx){others.push(null);continue;}
    const o=L.rooms[i];let cells=o.cells.filter(x=>!inRect(x));
    if(cells.length===o.cells.length){others.push(cells);continue;}
    if(cells.length&&!isRect(cells)){
      const [nr0,nc0,nr1,nc1]=bounds(o.cells);
      const cands=[[nr0,nc0,nr1,c0-1],[nr0,c1+1,nr1,nc1],[nr0,nc0,r0-1,nc1],[r1+1,nc0,nr1,nc1]]
        .map(([a,b,c,d])=>[Math.max(a,nr0),Math.max(b,nc0),Math.min(c,nr1),Math.min(d,nc1)])
        .filter(([a,b,c,d])=>a<=c&&b<=d);
      if(!cands.length)cells=[];
      else{const best=cands.reduce((p,q)=>((q[2]-q[0]+1)*(q[3]-q[1]+1)>(p[2]-p[0]+1)*(p[3]-p[1]+1)?q:p));
        cells=[];for(let r=best[0];r<=best[2];r++)for(let c=best[1];c<=best[3];c++)cells.push([r,c]);}
    }
    if(!cells.length){msg("Can't: room “"+(o.name||o.area||o.key)+"” would disappear");return false;}
    others.push(cells);
  }
  L.rooms.forEach((o,i)=>{if(i!==idx)o.cells=others[i];});
  L.rooms[idx].cells=rect;msg("");renderMap();return true;
}
window.addEventListener("resize",renderHandles);
// paint adds (r,c) to the selected room if the result is still a rectangle;
// alt-click removes a cell. Cells owned by another room are taken over.
function paint(r,c,remove){
  const rm=room();if(!rm)return;
  const mine=rm.cells.some(x=>x[0]===r&&x[1]===c);
  if(remove){
    if(!mine)return;
    const next=rm.cells.filter(x=>!(x[0]===r&&x[1]===c));
    if(next.length&&!isRect(next)){msg("Removing that cell would break the rectangle");return;}
    rm.cells=next;renderMap();return;
  }
  if(mine)return;
  const next=rm.cells.concat([[r,c]]);
  const grown=fillRect(next);
  state.layout.rooms.forEach((o,i)=>{if(i!==state.sel)o.cells=o.cells.filter(x=>!grown.some(g=>g[0]===x[0]&&g[1]===x[1]));});
  rm.cells=grown;msg("");renderMap();
}
function bounds(cells){let r0=1e9,c0=1e9,r1=-1,c1=-1;cells.forEach(([r,c])=>{r0=Math.min(r0,r);c0=Math.min(c0,c);r1=Math.max(r1,r);c1=Math.max(c1,c);});return [r0,c0,r1,c1];}
function isRect(cells){const [r0,c0,r1,c1]=bounds(cells);return (r1-r0+1)*(c1-c0+1)===cells.length;}
function fillRect(cells){const [r0,c0,r1,c1]=bounds(cells);const out=[];for(let r=r0;r<=r1;r++)for(let c=c0;c<=c1;c++)out.push([r,c]);return out;}

function renderRoom(){
  const rm=room();const inner=$("#inner"),list=$("#list");inner.innerHTML="";list.innerHTML="";
  if(!rm){$("#roomTitle").textContent="";return;}
  $("#roomTitle").textContent="— "+(rm.name||rm.area||rm.key);
  syncGrid(rm);
  $("#icols").value=rm.grid.columns;$("#irows").value=rm.grid.rows;$("#igridAuto").checked=!!rm.grid.auto;
  $("#icols").disabled=$("#irows").disabled=!!rm.grid.auto;
  inner.style.aspectRatio=String(innerAspect(rm));
  inner.style.gridTemplateColumns="repeat("+rm.grid.columns+",1fr)";inner.style.gridTemplateRows="repeat("+rm.grid.rows+",1fr)";
  const ents=areaOf(rm).entities;const byId={};ents.forEach(e=>byId[e.place_id]=e);
  const cr=(rm.grid.rows-1)/2,cc=(rm.grid.columns-1)/2;
  for(let r=0;r<rm.grid.rows;r++)for(let c=0;c<rm.grid.columns;c++){
    const cell=document.createElement("div");cell.className="icell"+(r===cr&&c===cc?" center":"");cell.dataset.r=r;cell.dataset.c=c;
    Object.entries(rm.entities).forEach(([id,pos])=>{if(pos[0]===r&&pos[1]===c){const e=byId[id]||{place_id:id,name:id,domain:"?",kind:"other"};cell.appendChild(chip(e,rm));}});
    dropzone(cell,id=>{rm.entities[id]=[r,c];renderRoom();});
    inner.appendChild(cell);
  }
  const showAll=$("#showAll").checked;
  ents.filter(e=>!(e.place_id in rm.entities)).filter(e=>showAll||["light","device","contact"].includes(e.kind)).forEach(e=>list.appendChild(chip(e,rm)));
  dropzone(list,id=>{delete rm.entities[id];renderRoom();});
}
function chip(e,rm){
  const el=document.createElement("span");el.className="ent"+(rm.hidden.includes(e.place_id)?" hidden":"");el.draggable=true;el.title=e.entity_id;
  el.innerHTML=(e.icon_svg||"")+'<span>'+esc(e.name)+'</span><span class="k">'+esc(e.kind)+'</span><button title="hide/show on the map">⊘</button>';
  el.addEventListener("dragstart",ev=>{ev.dataTransfer.setData("text/plain",e.place_id);ev.dataTransfer.effectAllowed="move";});
  el.querySelector("button").addEventListener("click",ev=>{ev.stopPropagation();const i=rm.hidden.indexOf(e.place_id);if(i>=0)rm.hidden.splice(i,1);else rm.hidden.push(e.place_id);renderRoom();});
  return el;
}
function dropzone(el,onDrop){
  el.addEventListener("dragover",ev=>{ev.preventDefault();el.classList.add("over");});
  el.addEventListener("dragleave",()=>el.classList.remove("over"));
  el.addEventListener("drop",ev=>{ev.preventDefault();el.classList.remove("over");const id=ev.dataTransfer.getData("text/plain");if(id)onDrop(id);});
}
function esc(s){return String(s??"").replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));}

$("#cols").addEventListener("change",e=>{state.layout.columns=+e.target.value||1;clampCells();renderAll();});
$("#rows").addEventListener("change",e=>{state.layout.rows=+e.target.value||1;clampCells();renderAll();});
$("#aspect").addEventListener("change",e=>{state.layout.aspect_ratio=e.target.value.trim();renderControls();});
$("#maxw").addEventListener("change",e=>{state.layout.max_width=+e.target.value||0;});
$("#icols").addEventListener("change",e=>{const rm=room();rm.grid.auto=false;rm.grid.columns=+e.target.value||1;clampInner(rm);renderRoom();});
$("#irows").addEventListener("change",e=>{const rm=room();rm.grid.auto=false;rm.grid.rows=+e.target.value||1;clampInner(rm);renderRoom();});
$("#igridAuto").addEventListener("change",e=>{const rm=room();rm.grid.auto=e.target.checked;renderRoom();});
$("#showAll").addEventListener("change",renderRoom);
function clampCells(){const L=state.layout;L.rooms.forEach(rm=>{rm.cells=rm.cells.filter(([r,c])=>r<L.rows&&c<L.columns);});}
function clampInner(rm){Object.entries(rm.entities).forEach(([id,[r,c]])=>{if(r>=rm.grid.rows||c>=rm.grid.columns)delete rm.entities[id];});}
$("#btnAddRoom").addEventListener("click",()=>{
  const L=state.layout;let key="room"+(L.rooms.length+1);while(L.rooms.some(r=>r.key===key))key+="x";
  let free=null;for(let r=0;r<L.rows&&!free;r++)for(let c=0;c<L.columns&&!free;c++)if(owner(r,c)<0)free=[r,c];
  L.rooms.push({key,area:"",name:"",cells:free?[free]:[],grid:{rows:3,columns:3},entities:{},hidden:[]});
  state.sel=L.rooms.length-1;renderAll();
});
$("#btnSave").addEventListener("click",async()=>{
  msg("Saving…");
  const r=await fetch(BASE+"/edit/layout.json",{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(state.layout)});
  const t=await r.text();
  if(!r.ok){msg(t.trim());return;}
  state.layout=normalize(JSON.parse(t));renderAll();msg("Saved. The dashboard picks it up on its next refresh.",true);
});
$("#btnPreview").addEventListener("click",async()=>{
  const r=await fetch(BASE+"/edit/preview",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(state.layout)});
  const t=await r.text();
  if(!r.ok){msg(t.trim());return;}
  msg("");$("#preview").hidden=false;$("#previewFrame").innerHTML=t;
  $("#preview").scrollIntoView({behavior:"smooth"});
});
$("#btnReset").addEventListener("click",()=>{
  if(!confirm("Discard the saved layout and reload the one from config? (Nothing is written until you press Save.)"))return;
  state.layout=normalize(JSON.parse(JSON.stringify(state.seed)));state.sel=0;renderAll();
});
load();
})();
</script>
</body>
</html>
`
