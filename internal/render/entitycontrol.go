package render

// entityControlCSS styles the click/long-press affordance on light and
// device tiles, and the floating popover a long-press opens (brightness,
// color temperature, color swatches for a light; speed and oscillation for
// a fan; target temperature and mode for climate / water heaters). It rides
// on the same Glance theme variables as the rest of the widget (see
// widgetCSS/nowPlayingCSS) so it follows light/dark and any custom theme.
const entityControlCSS = `
	.ha-light,.ha-device{touch-action:manipulation;-webkit-touch-callout:none;-webkit-user-select:none;user-select:none}
	.ha-light[data-fp-interactive="true"],.ha-device[data-fp-interactive="true"]{cursor:pointer}
	.ha-light[data-fp-interactive="true"]:hover svg,.ha-device[data-fp-interactive="true"]:hover svg{filter:brightness(1.35)}
	.ha-fp-icons>.ha-pressing>svg{animation:ha-pop-press .5s ease-out forwards}
	@keyframes ha-pop-press{to{transform:scale(.8)}}

	.ha-pop{position:fixed;z-index:1000;width:252px;padding:14px 15px;
	  border-radius:var(--border-radius,5px);background:var(--color-popover-background,var(--color-widget-background));
	  border:1px solid var(--color-popover-border,var(--color-widget-content-border));box-shadow:0 10px 30px rgba(0,0,0,.4);
	  display:flex;flex-direction:column;gap:13px;font-size:12px;color:var(--color-text-base);animation:ha-pop-in .14s ease-out;
	  max-height:calc(100vh - 16px);overflow-y:auto;overscroll-behavior:contain}
	@keyframes ha-pop-in{from{opacity:0;transform:translateY(4px)}}
	.ha-pop-head{display:flex;align-items:center;justify-content:space-between;gap:10px}
	.ha-pop-title{font-size:13px;font-weight:600;color:var(--color-text-highlight);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-pop-sub{font-size:11px;color:var(--color-text-subdue);margin-top:1px}
	.ha-pop-sw{width:34px;height:19px;border-radius:10px;border:0;padding:0;cursor:pointer;flex:none;
	  background:color-mix(in srgb,var(--color-text-base) 22%,transparent);transition:background .2s;position:relative}
	.ha-pop-sw::after{content:"";position:absolute;top:2px;left:2px;width:15px;height:15px;border-radius:50%;background:var(--color-text-highlight);transition:transform .2s}
	.ha-pop-sw[data-on="true"]{background:var(--color-primary)}
	.ha-pop-sw[data-on="true"]::after{transform:translateX(15px);background:var(--color-widget-background)}
	.ha-pop-row{display:flex;flex-direction:column;gap:6px}
	.ha-pop-line{display:flex;align-items:center;justify-content:space-between;gap:8px}
	.ha-pop-label{color:var(--color-text-subdue);font-size:10px;text-transform:uppercase;letter-spacing:.06em}
	.ha-pop-val{color:var(--color-text-highlight);font-variant-numeric:tabular-nums}
	.ha-pop-big{font-size:22px;font-weight:600;color:var(--color-text-highlight);font-variant-numeric:tabular-nums;min-width:4.5ch;text-align:center}
	.ha-pop-step{display:flex;align-items:center;justify-content:space-between;gap:8px}
	.ha-pop-btn{width:32px;height:32px;border-radius:50%;border:1px solid var(--color-widget-content-border);background:var(--color-widget-background-highlight,transparent);
	  color:var(--color-text-highlight);font:inherit;font-size:17px;line-height:1;cursor:pointer;padding:0}
	.ha-pop-btn:hover{border-color:var(--color-primary);color:var(--color-primary)}
	.ha-pop-chips{display:flex;flex-wrap:wrap;gap:5px}
	.ha-pop-chip{border:1px solid var(--color-widget-content-border);background:transparent;color:var(--color-text-base);border-radius:12px;
	  padding:3px 9px;font:inherit;font-size:11px;cursor:pointer}
	.ha-pop-chip[data-on="true"]{border-color:var(--color-primary);color:var(--color-primary);background:color-mix(in srgb,var(--color-primary) 12%,transparent)}
	.ha-pop-slider{width:100%;height:16px;margin:0;padding:0;border:0;border-radius:0;-webkit-appearance:none;appearance:none;background:transparent;cursor:pointer;touch-action:none}
	.ha-pop-slider::-webkit-slider-runnable-track{height:6px;border-radius:3px;
	  background:linear-gradient(90deg,var(--color-primary) var(--pct,0%),color-mix(in srgb,var(--color-text-base) 16%,transparent) var(--pct,0%))}
	.ha-pop-slider::-moz-range-track{height:6px;border-radius:3px;background:color-mix(in srgb,var(--color-text-base) 16%,transparent)}
	.ha-pop-slider::-moz-range-progress{height:6px;border-radius:3px;background:var(--color-primary)}
	.ha-pop-slider::-webkit-slider-thumb{-webkit-appearance:none;width:16px;height:16px;border-radius:50%;background:var(--color-text-highlight);margin-top:-5px;border:0;box-shadow:0 1px 4px rgba(0,0,0,.4)}
	.ha-pop-slider::-moz-range-thumb{width:16px;height:16px;border-radius:50%;background:var(--color-text-highlight);border:0;box-shadow:0 1px 4px rgba(0,0,0,.4)}
	.ha-pop-slider.ha-pop-ct::-webkit-slider-runnable-track{background:linear-gradient(90deg,#ffb35c,#ffe9c7,#e6f0ff,#b9d4ff)}
	.ha-pop-slider.ha-pop-ct::-moz-range-track{background:linear-gradient(90deg,#ffb35c,#ffe9c7,#e6f0ff,#b9d4ff)}
	.ha-pop-slider.ha-pop-ct::-moz-range-progress{background:transparent}
	.ha-pop-swatches{display:flex;flex-wrap:wrap;gap:6px}
	.ha-pop-swatch{width:22px;height:22px;border-radius:50%;border:2px solid transparent;cursor:pointer;padding:0;box-shadow:inset 0 0 0 1px rgba(255,255,255,.2)}
	.ha-pop-swatch[data-on="true"]{border-color:var(--color-text-highlight)}
	.ha-pop-sep{height:1px;background:var(--color-widget-content-border);margin:0 -15px;flex:none}
	.ha-pop-sel{width:100%;height:28px;border-radius:var(--border-radius,5px);border:1px solid var(--color-widget-content-border);background:transparent;color:var(--color-text-base);font:inherit;font-size:12px;padding:0 6px}
	.ha-pop-err{color:var(--color-negative);font-size:11px}
`

// entityControlScript is a second onerror-driven IIFE, appended after
// bootstrapCore (see template.go), sharing nothing with it but the DOM: it
// re-derives root from the same img element, and relies on bootstrapCore's
// poll loop (via the data-hold-until guard) to reconcile state rather than
// triggering its own.
//
// Click vs. long-press is decided with pointer events (unifies mouse and
// touch — this widget is as likely to run on a wall-mounted touchscreen as
// a desktop browser): a short tap toggles the entity; holding for
// LONG_PRESS_MS opens the popover for entities that have one; movement past
// MOVE_TOLERANCE cancels both, so a scroll starting on a tile is never
// mistaken for a press. Only the primary mouse button counts, and the
// long-press context menu (iOS callout / Android menu) is suppressed.
const entityControlScript = `;(function(img){
	var root=img.closest('.ha-widget');if(!root)return;

	// fanVisuals turns a fan tile's live data into the CSS the air stream
	// reads (see floorplanCSS): --spd/--spdt from the speed, data-breeze
	// for natural-wind presets, data-osc and --sweep (from the device's
	// angle select, if any) for oscillation. It re-runs whenever the poller
	// or the popover changes those attributes, so the map follows at once.
	function fanVisuals(t){
		var d=t.dataset;
		if(d.hasSpeed==='true'){var p=Math.max(0,Math.min(100,+d.percentage||0))/100;t.style.setProperty('--spd',(0.35+0.65*p).toFixed(3));t.style.setProperty('--spdt',p.toFixed(3));}
		var breeze=/natur|breeze|nature|sleep/i.test(d.presetMode||'');
		if(d.breeze!==String(breeze))d.breeze=String(breeze);
		var osc=d.oscillating==='true';
		if(d.osc!==String(osc))d.osc=String(osc);
		var angle=0;
		try{
			var ex=JSON.parse(d.extras||'[]'),st=JSON.parse(d.extraStates||'{}');
			ex.forEach(function(e){if(e.domain==='select'&&/angle/i.test(e.name+e.id)){var v=parseFloat(st[e.id]!=null?st[e.id]:e.state);if(v)angle=v;}});
		}catch(_){}
		var want=angle?Math.min(45,Math.max(8,angle/4)):30;
		t.style.setProperty('--sweep',fitSweep(t,want)+'deg');
	}
	// fitSweep narrows the oscillation so the stream never swings into a
	// wall: the tip of each visible cone edge, at the far ends of the sweep,
	// must stay inside the room. Geometry mirrors floorplanCSS: the stream
	// starts at the icon, points at the room centre, is len·reach·spd long,
	// and its visible body is about ±0.36 of its length wide (the cone scales
	// as a whole, so its opening angle doesn't depend on the speed).
	function fitSweep(t,want){
		var room=t.closest('.ha-fp-room');if(!room||t.dataset.center==='true')return want;
		var R=room.getBoundingClientRect(),I=t.getBoundingClientRect();if(!R.width||!I.width)return want;
		var px=I.left+I.width/2,py=I.top+I.height/2,vx=R.left+R.width/2-px,vy=R.top+R.height/2-py;
		var len=Math.hypot(vx,vy);if(!len)return want;
		var cs=getComputedStyle(t),reach=parseFloat(cs.getPropertyValue('--reach'))||.92,spd=parseFloat(t.style.getPropertyValue('--spd'))||1;
		var L=len*reach*spd,half=Math.atan(0.36/reach),dir=Math.atan2(-vx,vy),m=4;
		function inside(a){var x=px-Math.sin(a)*L,y=py+Math.cos(a)*L;return x>=R.left+m&&x<=R.right-m&&y>=R.top+m&&y<=R.bottom-m;}
		for(var s=want;s>0;s-=1){
			var r=s*Math.PI/180;
			if(inside(dir+r+half)&&inside(dir-r-half)&&inside(dir+r-half)&&inside(dir-r+half))return s;
		}
		return 0;
	}
	root.querySelectorAll('.ha-device[data-effect="fan"]').forEach(function(t){
		fanVisuals(t);
		new MutationObserver(function(){fanVisuals(t);}).observe(t,{attributes:true,attributeFilter:['data-percentage','data-preset-mode','data-oscillating','data-extra-states']});
		var room=t.closest('.ha-fp-room');
		if(room&&window.ResizeObserver)new ResizeObserver(function(){fanVisuals(t);}).observe(room);
	});

	var entityUrl=root.dataset.entityUrl;if(!entityUrl)return;

	var TOGGLE_DOMAINS={light:true,switch:true,fan:true,cover:true,lock:true,humidifier:true,water_heater:true,vacuum:true,climate:true};
	var MODE_LABELS={off:'Off',heat:'Heat',cool:'Cool',heat_cool:'Heat/Cool',auto:'Auto',dry:'Dry',fan_only:'Fan'};
	var SWATCHES=['#ff3b30','#ff9500','#ffcc00','#34c759','#30d5c8','#0a84ff','#af52de','#ff2d78'];

	function eligibleTile(el){
		var tile=el.closest&&el.closest('.ha-light,.ha-device');
		if(!tile||!root.contains(tile))return null;
		if(tile.classList.contains('ha-device')&&!TOGGLE_DOMAINS[tile.dataset.domain])return null;
		return tile;
	}
	function canPopover(t){
		var d=t.dataset;
		if(t.classList.contains('ha-light'))return d.hasBrightness==='true'||d.hasColorTemp==='true'||d.hasColor==='true';
		return d.hasTargetTemp==='true'||d.hasSpeed==='true'||d.hasOscillate==='true'||!!d.hvacModes||!!d.presetModes||!!d.extras;
	}
	root.querySelectorAll('.ha-light,.ha-device').forEach(function(t){if(eligibleTile(t)===t)t.dataset.fpInteractive='true';});

	function esc(s){return String(s==null?'':s).replace(/[&<>"]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];});}
	function hold(tile){tile.dataset.holdUntil=String(Date.now()+6000);}
	function post(action,entityId,data){
		var body=Object.assign({entity_id:entityId,action:action},data||{});
		return fetch(entityUrl,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'text/plain'},body:JSON.stringify(body)})
			.then(function(r){if(!r.ok)return r.text().then(function(t){throw new Error((t||r.status+'').trim());});return r;});
	}
	function showErr(msg){
		if(!pop)return;
		var e=pop.querySelector('.ha-pop-err');
		if(!e){e=document.createElement('div');e.className='ha-pop-err';pop.appendChild(e);}
		e.textContent=msg;
	}
	function setOn(tile,on){tile.dataset.on=String(on);if(pop&&popFor===tile){var sw=pop.querySelector('.ha-pop-head .ha-pop-sw');if(sw)sw.dataset.on=String(on);}}
	function toggleTile(tile){
		var was=tile.dataset.on==='true';
		setOn(tile,!was);hold(tile);
		post('toggle',tile.dataset.entityId).catch(function(err){setOn(tile,was);delete tile.dataset.holdUntil;showErr(err.message);});
	}

	var pop=null,popFor=null;
	function onKey(e){if(e.key==='Escape')closePopover();}
	function onOutside(e){if(pop&&!pop.contains(e.target)&&!popFor.contains(e.target))closePopover();}
	function closePopover(){
		if(!pop)return;
		pop.remove();pop=null;popFor=null;
		document.removeEventListener('keydown',onKey);
		document.removeEventListener('pointerdown',onOutside,true);
		window.removeEventListener('scroll',closePopover,true);
	}
	function pct(v,min,max){return Math.max(0,Math.min(100,Math.round(100*(v-min)/((max-min)||1))));}
	function slider(field,min,max,step,val,cls){
		return '<input type="range" class="ha-pop-slider'+(cls?' '+cls:'')+'" data-field="'+field+'" min="'+min+'" max="'+max+'" step="'+step+'" value="'+val+'" style="--pct:'+pct(val,min,max)+'%">';
	}
	function row(label,valHtml,body){
		return '<div class="ha-pop-row"><div class="ha-pop-line"><span class="ha-pop-label">'+label+'</span><span class="ha-pop-val">'+valHtml+'</span></div>'+body+'</div>';
	}
	function deg(v){return (+v).toFixed(1).replace(/\.0$/,'')+'°';}

	function parse(v,dflt){try{return JSON.parse(v);}catch(_){return dflt;}}
	// extras: the device's sibling select/number/switch entities; states in
	// data-extra-states (kept fresh by the poller) win over the rendered ones
	function extras(tile){
		var list=parse(tile.dataset.extras||'[]',[]),st=parse(tile.dataset.extraStates||'{}',{});
		list.forEach(function(e){if(st[e.id]!=null)e.state=st[e.id];});
		return list;
	}
	function setExtraState(tile,id,v){var st=parse(tile.dataset.extraStates||'{}',{});st[id]=String(v);tile.dataset.extraStates=JSON.stringify(st);hold(tile);}
	function numLabel(v,e){
		if(e.unit==='minutes'||e.unit==='min'){if(!v)return 'Off';var h=Math.floor(v/60),m=Math.round(v%60);return (h?h+' h ':'')+(m||!h?m+' min':'');}
		return (Math.round(v*100)/100)+(e.unit?' '+esc(e.unit):'');
	}
	function extrasHtml(tile){
		var list=extras(tile);if(!list.length)return '';
		var html='<div class="ha-pop-sep"></div>';
		list.forEach(function(e){
			if(e.domain==='select'){
				var deg=/angle/i.test(e.name)&&e.options.every(function(o){return /^\d+$/.test(o);})?'°':'';
				var short=e.options.length<=6&&e.options.join('').length<=40;
				// "Level1".."Level4" → chips "1".."4": drop a word prefix all options share
				var pre=(e.options[0].match(/^[A-Za-z ]+(?=\d)/)||[''])[0];
				if(!pre||!e.options.every(function(o){return o.indexOf(pre)===0&&/^\d+$/.test(o.slice(pre.length));}))pre='';
				html+='<div class="ha-pop-row"><span class="ha-pop-label">'+esc(e.name)+'</span>'+(short?
					'<div class="ha-pop-chips">'+e.options.map(function(o){return '<button type="button" class="ha-pop-chip" data-extra="'+esc(e.id)+'" data-option="'+esc(o)+'" data-on="'+(o===e.state)+'">'+esc(o.slice(pre.length))+deg+'</button>';}).join('')+'</div>':
					'<select class="ha-pop-sel" data-extra="'+esc(e.id)+'">'+e.options.map(function(o){return '<option'+(o===e.state?' selected':'')+'>'+esc(o)+'</option>';}).join('')+'</select>')+'</div>';
			}else if(e.domain==='number'){
				var v=+e.state||0;e.min=e.min||0;e.max=e.max||0;
				html+='<div class="ha-pop-row"><div class="ha-pop-line"><span class="ha-pop-label">'+esc(e.name)+'</span><span class="ha-pop-val" data-out-extra="'+esc(e.id)+'">'+numLabel(v,e)+'</span></div>'+
					'<input type="range" class="ha-pop-slider" data-extra="'+esc(e.id)+'" min="'+e.min+'" max="'+e.max+'" step="'+(e.step||1)+'" value="'+v+'" style="--pct:'+pct(v,e.min,e.max)+'%"></div>';
			}else if(e.domain==='switch'){
				html+='<div class="ha-pop-line"><span class="ha-pop-label">'+esc(e.name)+'</span><button type="button" class="ha-pop-sw" data-extra="'+esc(e.id)+'" data-on="'+(e.state==='on')+'" aria-label="'+esc(e.name)+'"></button></div>';
			}
		});
		return html;
	}

	function buildPanel(tile){
		var d=tile.dataset,isLight=tile.classList.contains('ha-light');
		var panel=document.createElement('div');
		panel.className='ha-pop';panel.setAttribute('role','dialog');
		var name=tile.getAttribute('title')||d.entityId;
		var sub=d.hasTargetTemp==='true'&&d.currentTemp&&+d.currentTemp?'Now '+deg(d.currentTemp):'';
		var html='<div class="ha-pop-head"><div style="min-width:0"><div class="ha-pop-title">'+esc(name)+'</div>'+(sub?'<div class="ha-pop-sub">'+sub+'</div>':'')+'</div>'+
			'<button type="button" class="ha-pop-sw" data-act="power" data-on="'+(d.on==='true')+'" aria-label="On/off"></button></div>';
		if(isLight){
			if(d.hasBrightness==='true'){var b=Math.max(1,+d.brightness||1);html+=row('Brightness','<span data-out="brightness">'+b+'%</span>',slider('brightness',1,100,1,b));}
			if(d.hasColorTemp==='true'){
				var ctMin=+d.colorTempMin||2000,ctMax=+d.colorTempMax||6500,ct=+d.colorTemp||Math.round((ctMin+ctMax)/2);
				html+=row('Tone','<span data-out="color_temp">'+ct+'K</span>',slider('color_temp',ctMin,ctMax,50,ct,'ha-pop-ct'));
			}
			if(d.hasColor==='true'){
				// mark the one swatch nearest the light's current color, if any is close
				var cur=(d.rgb||'').split(',').map(Number),best=-1,bestDist=60;
				var rgbs=SWATCHES.map(function(c){return [1,3,5].map(function(i){return parseInt(c.slice(i,i+2),16);});});
				if(cur.length===3)rgbs.forEach(function(rgb,i){var dist=Math.abs(cur[0]-rgb[0])+Math.abs(cur[1]-rgb[1])+Math.abs(cur[2]-rgb[2]);if(dist<bestDist){bestDist=dist;best=i;}});
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Color</span><div class="ha-pop-swatches">'+SWATCHES.map(function(c,i){
					return '<button type="button" class="ha-pop-swatch" data-rgb="'+rgbs[i].join(',')+'" data-on="'+(i===best)+'" style="background:'+c+'" aria-label="'+c+'"></button>';
				}).join('')+'</div></div>';
			}
		}else{
			if(d.hasTargetTemp==='true'){
				var min=+d.minTemp||7,max=+d.maxTemp||35,step=+d.tempStep||0.5,tgt=+d.targetTemp||min;
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Target</span><div class="ha-pop-step">'+
					'<button type="button" class="ha-pop-btn" data-act="temp-" aria-label="Lower">−</button><span class="ha-pop-big" data-out="temperature">'+deg(tgt)+'</span>'+
					'<button type="button" class="ha-pop-btn" data-act="temp+" aria-label="Higher">+</button></div>'+slider('temperature',min,max,step,tgt)+'</div>';
			}
			if(d.hvacModes){
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Mode</span><div class="ha-pop-chips">'+d.hvacModes.split(',').map(function(m){
					return '<button type="button" class="ha-pop-chip" data-mode="'+esc(m)+'" data-on="'+(m===d.hvacMode)+'">'+esc(MODE_LABELS[m]||m)+'</button>';
				}).join('')+'</div></div>';
			}
			if(d.hasSpeed==='true'){
				var st=+d.percentageStep||1,p=+d.percentage||0;
				html+=row('Speed','<span data-out="percentage">'+(p?Math.round(p)+'%':'Off')+'</span>',slider('percentage',0,100,st,p));
			}
			if(d.presetModes){
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Mode</span><div class="ha-pop-chips">'+parse(d.presetModes,[]).map(function(m){
					return '<button type="button" class="ha-pop-chip" data-preset="'+esc(m)+'" data-on="'+(m===d.presetMode)+'">'+esc(m)+'</button>';
				}).join('')+'</div></div>';
			}
			if(d.hasOscillate==='true'){
				html+='<div class="ha-pop-line"><span class="ha-pop-label">Oscillate</span><button type="button" class="ha-pop-sw" data-act="oscillate" data-on="'+(d.oscillating==='true')+'" aria-label="Oscillate"></button></div>';
			}
			html+=extrasHtml(tile);
		}
		panel.innerHTML=html;
		return panel;
	}

	function positionPanel(panel,tile){
		document.body.appendChild(panel);
		var r=tile.getBoundingClientRect(),pw=panel.offsetWidth,ph=panel.offsetHeight;
		var x=Math.min(Math.max(8,r.left+r.width/2-pw/2),window.innerWidth-pw-8);
		var y=r.top-ph-10;
		if(y<8)y=Math.min(r.bottom+10,window.innerHeight-ph-8);
		panel.style.left=x+'px';
		panel.style.top=Math.max(8,y)+'px';
	}

	var timers={};
	var FIELD_ACTIONS={brightness:['set_brightness','brightness_pct'],color_temp:['set_color_temp','color_temp_kelvin'],temperature:['set_temperature','temperature'],percentage:['set_percentage','percentage']};
	function send(tile,field,value,immediate){
		hold(tile);
		var key=tile.dataset.entityId+':'+field;
		clearTimeout(timers[key]);
		var run=function(){var a=FIELD_ACTIONS[field],data={};data[a[1]]=value;post(a[0],tile.dataset.entityId,data).catch(function(err){showErr(err.message);});};
		if(immediate)run();else timers[key]=setTimeout(run,250);
	}
	// reflect a value in the tile's dataset (so the popover reopens with it)
	// and in the panel's readout
	function reflect(panel,tile,field,v){
		var d=tile.dataset,out=panel.querySelector('[data-out="'+field+'"]');
		if(field==='brightness'){d.brightness=v;if(out)out.textContent=v+'%';setOn(tile,true);}
		else if(field==='color_temp'){d.colorTemp=v;if(out)out.textContent=v+'K';setOn(tile,true);}
		else if(field==='temperature'){d.targetTemp=v;if(out)out.textContent=deg(v);}
		else if(field==='percentage'){d.percentage=v;if(out)out.textContent=v?Math.round(v)+'%':'Off';setOn(tile,v>0);}
	}

	function wirePanel(panel,tile){
		panel.addEventListener('click',function(e){
			var btn=e.target.closest('button');if(!btn)return;
			var act=btn.dataset.act;
			if(btn.dataset.preset){
				var pm=btn.dataset.preset;
				btn.parentNode.querySelectorAll('.ha-pop-chip').forEach(function(c){c.dataset.on=String(c===btn);});
				tile.dataset.presetMode=pm;hold(tile);
				post('set_preset_mode',tile.dataset.entityId,{preset_mode:pm}).catch(function(err){showErr(err.message);});return;
			}
			if(btn.dataset.extra&&btn.dataset.option!=null){
				var opt=btn.dataset.option;
				btn.parentNode.querySelectorAll('.ha-pop-chip').forEach(function(c){c.dataset.on=String(c===btn);});
				setExtraState(tile,btn.dataset.extra,opt);
				post('select_option',btn.dataset.extra,{option:opt}).catch(function(err){showErr(err.message);});return;
			}
			if(btn.dataset.extra){
				var on2=btn.dataset.on!=='true';btn.dataset.on=String(on2);setExtraState(tile,btn.dataset.extra,on2?'on':'off');
				post('toggle',btn.dataset.extra).catch(function(err){btn.dataset.on=String(!on2);showErr(err.message);});return;
			}
			if(act==='power'){toggleTile(tile);return;}
			if(act==='oscillate'){
				var on=btn.dataset.on!=='true';btn.dataset.on=String(on);tile.dataset.oscillating=String(on);hold(tile);
				post('oscillate',tile.dataset.entityId,{oscillating:on}).catch(function(err){btn.dataset.on=String(!on);showErr(err.message);});return;
			}
			if(act==='temp-'||act==='temp+'){
				var s=panel.querySelector('[data-field="temperature"]');if(!s)return;
				var st=+s.step||0.5,v=Math.round((+s.value+(act==='temp+'?st:-st))/st)*st;
				v=Math.max(+s.min,Math.min(+s.max,v));s.value=v;s.style.setProperty('--pct',pct(v,+s.min,+s.max)+'%');
				reflect(panel,tile,'temperature',v);send(tile,'temperature',v,false);return;
			}
			if(btn.dataset.mode){
				var mode=btn.dataset.mode;
				panel.querySelectorAll('.ha-pop-chip').forEach(function(c){c.dataset.on=String(c===btn);});
				tile.dataset.hvacMode=mode;setOn(tile,mode!=='off');hold(tile);
				post('set_hvac_mode',tile.dataset.entityId,{hvac_mode:mode}).catch(function(err){showErr(err.message);});return;
			}
			if(btn.dataset.rgb){
				var rgb=btn.dataset.rgb.split(',').map(Number);
				panel.querySelectorAll('.ha-pop-swatch').forEach(function(c){c.dataset.on=String(c===btn);});
				tile.dataset.rgb=btn.dataset.rgb;setOn(tile,true);hold(tile);
				post('set_color',tile.dataset.entityId,{rgb_color:rgb}).catch(function(err){showErr(err.message);});
			}
		});
		var extraTimers={};
		panel.querySelectorAll('[data-extra]').forEach(function(el){
			if(el.tagName==='SELECT')el.addEventListener('change',function(){
				setExtraState(tile,el.dataset.extra,el.value);
				post('select_option',el.dataset.extra,{option:el.value}).catch(function(err){showErr(err.message);});
			});
			if(el.tagName!=='INPUT')return;
			var e=extras(tile).filter(function(x){return x.id===el.dataset.extra;})[0]||{};
			var sendVal=function(){post('set_value',el.dataset.extra,{value:+el.value}).catch(function(err){showErr(err.message);});};
			el.addEventListener('input',function(){
				el.style.setProperty('--pct',pct(+el.value,+el.min,+el.max)+'%');
				var o=panel.querySelector('[data-out-extra="'+CSS.escape(el.dataset.extra)+'"]');if(o)o.innerHTML=numLabel(+el.value,e);
				setExtraState(tile,el.dataset.extra,el.value);
				clearTimeout(extraTimers[el.dataset.extra]);extraTimers[el.dataset.extra]=setTimeout(sendVal,400);
			});
			el.addEventListener('change',function(){clearTimeout(extraTimers[el.dataset.extra]);sendVal();});
		});
		panel.querySelectorAll('.ha-pop-slider:not([data-extra])').forEach(function(input){
			input.addEventListener('input',function(){
				var v=+input.value;
				input.style.setProperty('--pct',pct(v,+input.min,+input.max)+'%');
				reflect(panel,tile,input.dataset.field,v);
				send(tile,input.dataset.field,v,false);
			});
			input.addEventListener('change',function(){send(tile,input.dataset.field,+input.value,true);});
		});
	}

	function openPopover(tile){
		if(popFor===tile){closePopover();return;}
		closePopover();
		var panel=buildPanel(tile);
		popFor=tile;pop=panel;
		positionPanel(panel,tile);
		wirePanel(panel,tile);
		document.addEventListener('keydown',onKey);
		document.addEventListener('pointerdown',onOutside,true);
		window.addEventListener('scroll',closePopover,true);
	}

	var press=null,LONG_PRESS_MS=500,MOVE_TOLERANCE=10;
	function cancelPress(){if(!press)return;clearTimeout(press.timer);press.tile.classList.remove('ha-pressing');press=null;}
	root.addEventListener('pointerdown',function(e){
		if(e.pointerType==='mouse'&&e.button!==0)return;
		var tile=eligibleTile(e.target);
		if(!tile)return;
		cancelPress();
		press={tile:tile,x:e.clientX,y:e.clientY};
		if(canPopover(tile))tile.classList.add('ha-pressing');
		press.timer=setTimeout(function(){
			if(!press)return;
			var t=press.tile;cancelPress();
			if(canPopover(t)){openPopover(t);if(navigator.vibrate)navigator.vibrate(10);}
			else toggleTile(t);
		},LONG_PRESS_MS);
	});
	root.addEventListener('pointermove',function(e){
		if(press&&(Math.abs(e.clientX-press.x)>MOVE_TOLERANCE||Math.abs(e.clientY-press.y)>MOVE_TOLERANCE))cancelPress();
	});
	root.addEventListener('pointerup',function(){
		if(!press)return;
		var tile=press.tile;cancelPress();
		toggleTile(tile);
	});
	root.addEventListener('pointercancel',cancelPress);
	root.addEventListener('contextmenu',function(e){if(eligibleTile(e.target))e.preventDefault();});
})(this)`
