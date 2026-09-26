package render

// tempChartCSS styles the room-temperature popover (a click on a room's
// temperature chip on the map). It reuses the .ha-pop shell from
// entityControlCSS and only adds the chart.
const tempChartCSS = `
	.ha-widget[data-temp-url]:not([data-temp-url=""]) .ha-fp-temp{cursor:pointer;transition:background .15s}
	.ha-widget[data-temp-url]:not([data-temp-url=""]) .ha-fp-temp:hover{background:color-mix(in srgb,var(--color-primary) 18%,transparent)}
	.ha-pop.ha-tpop{width:320px}
	.ha-tpop-stats{display:flex;gap:14px;font-size:11px;color:var(--color-text-subdue)}
	.ha-tpop-stats b{color:var(--color-text-highlight);font-weight:600;font-variant-numeric:tabular-nums}
	.ha-tpop-chart{position:relative;height:130px;touch-action:none}
	.ha-tpop-chart svg{position:absolute;inset:0;width:100%;height:100%;overflow:visible}
	.ha-tpop-line{fill:none;stroke:var(--color-primary);stroke-width:2;vector-effect:non-scaling-stroke;stroke-linejoin:round;stroke-linecap:round}
	.ha-tpop-grid{stroke:var(--color-widget-content-border);stroke-width:1;vector-effect:non-scaling-stroke}
	.ha-tpop-ylab{position:absolute;right:0;font-size:10px;color:var(--color-text-subdue);transform:translateY(-50%);background:var(--color-popover-background,var(--color-widget-background));padding-left:3px}
	.ha-tpop-x{display:flex;justify-content:space-between;font-size:10px;color:var(--color-text-subdue);margin-top:4px}
	.ha-tpop-cursor{position:absolute;inset-block:0;width:1px;background:var(--color-text-subdue);pointer-events:none;display:none}
	.ha-tpop-dot{position:absolute;width:8px;height:8px;margin:-4px 0 0 -4px;border-radius:50%;background:var(--color-primary);box-shadow:0 0 0 3px color-mix(in srgb,var(--color-primary) 30%,transparent);pointer-events:none;display:none}
	.ha-tpop-empty{height:130px;display:flex;align-items:center;justify-content:center;color:var(--color-text-subdue)}
`

// tempChartScript is a third onerror-driven IIFE (after bootstrapCore and
// entityControlScript): a click on a room's temperature chip fetches the
// room's last 12 hours from data-temp-url and draws them in a popover —
// a line over a soft fill, min/max/now, a time axis every 3 hours, and a
// crosshair whose value and time replace the stats line while the pointer
// (mouse or touch) is over the chart.
const tempChartScript = `;(function(img){
	var root=img.closest('.ha-widget');if(!root)return;
	var url=root.dataset.tempUrl;if(!url)return;
	var pop=null,popFor=null;
	function close(){if(!pop)return;pop.remove();pop=null;popFor=null;document.removeEventListener('pointerdown',outside,true);document.removeEventListener('keydown',key);window.removeEventListener('scroll',close,true);}
	function outside(e){if(pop&&!pop.contains(e.target)&&!popFor.contains(e.target))close();}
	function key(e){if(e.key==='Escape')close();}
	function esc(s){return String(s==null?'':s).replace(/[&<>"]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];});}
	function hm(t){var d=new Date(t);return String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0');}
	function deg(v){return v.toFixed(1)+'°';}
	function place(panel,anchor){
		document.body.appendChild(panel);
		var r=anchor.getBoundingClientRect(),pw=panel.offsetWidth,ph=panel.offsetHeight;
		var x=Math.min(Math.max(8,r.left+r.width/2-pw/2),window.innerWidth-pw-8),y=r.bottom+8;
		if(y+ph>window.innerHeight-8)y=Math.max(8,r.top-ph-8);
		panel.style.left=x+'px';panel.style.top=y+'px';
	}
	function draw(panel,data){
		var pts=(data.points||[]).filter(function(p){return p.v!=null;});
		var body=panel.querySelector('.ha-tpop-body');
		if(pts.length<2){body.innerHTML='<div class="ha-tpop-empty">No data for the last 12 hours</div>';return;}
		var t0=data.points[0].t,t1=data.points[data.points.length-1].t;
		var vs=pts.map(function(p){return p.v;}),lo=Math.min.apply(null,vs),hi=Math.max.apply(null,vs),now=vs[vs.length-1];
		var pad=Math.max(0.5,(hi-lo)*0.15),y0=lo-pad,y1=hi+pad;
		var W=1000,H=100,X=function(t){return (t-t0)/((t1-t0)||1)*W;},Y=function(v){return H-(v-y0)/((y1-y0)||1)*H;};
		var line=pts.map(function(p,i){return (i?'L':'M')+X(p.t).toFixed(1)+' '+Y(p.v).toFixed(1);}).join(' ');
		var area=line+' L'+X(pts[pts.length-1].t).toFixed(1)+' '+H+' L'+X(pts[0].t).toFixed(1)+' '+H+' Z';
		var gid='ha-tg-'+Math.random().toString(36).slice(2,8);
		var stats=panel.querySelector('.ha-tpop-stats');
		var statsHtml='<span>now <b>'+deg(now)+'</b></span><span>min <b>'+deg(lo)+'</b></span><span>max <b>'+deg(hi)+'</b></span>';
		stats.innerHTML=statsHtml;
		var xs=[];for(var i=0;i<=4;i++)xs.push('<span>'+(i===4?'now':hm(t0+(t1-t0)*i/4))+'</span>');
		body.innerHTML='<div class="ha-tpop-chart"><svg viewBox="0 0 '+W+' '+H+'" preserveAspectRatio="none">'+
			'<defs><linearGradient id="'+gid+'" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:var(--color-primary);stop-opacity:.32"/><stop offset="1" style="stop-color:var(--color-primary);stop-opacity:0"/></linearGradient></defs>'+
			'<line class="ha-tpop-grid" x1="0" x2="'+W+'" y1="'+Y(hi)+'" y2="'+Y(hi)+'"/><line class="ha-tpop-grid" x1="0" x2="'+W+'" y1="'+Y(lo)+'" y2="'+Y(lo)+'"/>'+
			'<path d="'+area+'" fill="url(#'+gid+')" stroke="none"/><path class="ha-tpop-line" d="'+line+'"/></svg>'+
			'<span class="ha-tpop-ylab" style="top:'+Y(hi)+'%">'+deg(hi)+'</span><span class="ha-tpop-ylab" style="top:'+Y(lo)+'%">'+deg(lo)+'</span>'+
			'<span class="ha-tpop-cursor"></span><span class="ha-tpop-dot"></span></div><div class="ha-tpop-x">'+xs.join('')+'</div>';
		var chart=body.querySelector('.ha-tpop-chart'),cur=chart.querySelector('.ha-tpop-cursor'),dot=chart.querySelector('.ha-tpop-dot');
		function show(e){
			var r=chart.getBoundingClientRect(),t=t0+Math.max(0,Math.min(1,(e.clientX-r.left)/r.width))*(t1-t0),best=pts[0];
			pts.forEach(function(p){if(Math.abs(p.t-t)<Math.abs(best.t-t))best=p;});
			var x=X(best.t)/W*100,y=Y(best.v)/H*100;
			cur.style.left=dot.style.left=x+'%';dot.style.top=y+'%';
			stats.innerHTML='<span>at '+hm(best.t)+' <b>'+deg(best.v)+'</b></span>';
			cur.style.display=dot.style.display='block';
		}
		function hide(){cur.style.display=dot.style.display='none';stats.innerHTML=statsHtml;}
		chart.addEventListener('pointermove',show);chart.addEventListener('pointerdown',show);chart.addEventListener('pointerleave',hide);
	}
	root.addEventListener('click',function(e){
		var chip=e.target.closest('.ha-fp-temp');if(!chip||!root.contains(chip))return;
		var room=chip.closest('.ha-fp-room');if(!room)return;
		if(popFor===chip){close();return;}
		close();
		var name=(room.querySelector('.ha-fp-name')||{}).textContent||room.dataset.room;
		var panel=document.createElement('div');panel.className='ha-pop ha-tpop';panel.setAttribute('role','dialog');
		panel.innerHTML='<div class="ha-pop-head"><div style="min-width:0"><div class="ha-pop-title">'+esc(name)+'</div><div class="ha-pop-sub">Temperature · last 12 hours</div></div></div>'+
			'<div class="ha-tpop-stats"></div><div class="ha-tpop-body"><div class="ha-tpop-empty">Loading…</div></div>';
		pop=panel;popFor=chip;place(panel,chip);
		document.addEventListener('pointerdown',outside,true);document.addEventListener('keydown',key);window.addEventListener('scroll',close,true);
		fetch(url+(url.indexOf('?')<0?'?':'&')+'room='+encodeURIComponent(room.dataset.room),{cache:'no-store'})
			.then(function(r){if(!r.ok)throw new Error(r.status);return r.json();})
			.then(function(d){if(pop===panel)draw(panel,d);})
			.catch(function(err){if(pop===panel)panel.querySelector('.ha-tpop-body').innerHTML='<div class="ha-tpop-empty">Could not load: '+esc(err.message)+'</div>';});
	});
})(this)`
