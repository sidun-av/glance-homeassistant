package render

// entityControlCSS styles the click/long-press affordance on light and
// device tiles, and the floating popover a long-press opens (brightness,
// color temperature, color swatches for a light; target temperature for
// climate). It rides on the same Glance theme variables as the rest of the
// widget (see widgetCSS/nowPlayingCSS) so it follows light/dark and any
// custom theme automatically.
const entityControlCSS = `
	.ha-light,.ha-device{touch-action:manipulation;-webkit-touch-callout:none;user-select:none}
	.ha-light[data-fp-interactive="true"],.ha-device[data-fp-interactive="true"]{cursor:pointer}

	.ha-pop{position:fixed;z-index:1000;min-width:190px;max-width:260px;padding:12px;
	  border-radius:var(--border-radius,8px);background:var(--color-widget-background-highlight);
	  border:1px solid var(--color-widget-content-border);box-shadow:0 8px 24px rgba(0,0,0,.35);
	  display:flex;flex-direction:column;gap:10px;font-size:12px}
	.ha-pop-head{display:flex;align-items:center;justify-content:space-between;gap:10px}
	.ha-pop-title{font-weight:600;color:var(--color-text-highlight);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-pop-toggle{width:34px;height:19px;border-radius:10px;border:0;padding:0;cursor:pointer;flex:none;
	  background:color-mix(in srgb,var(--color-text-base) 22%,transparent);transition:background .2s;position:relative}
	.ha-pop-toggle::after{content:"";position:absolute;top:2px;left:2px;width:15px;height:15px;border-radius:50%;background:#fff;transition:transform .2s}
	.ha-pop-toggle[data-on="true"]{background:var(--color-primary)}
	.ha-pop-toggle[data-on="true"]::after{transform:translateX(15px)}
	.ha-pop-row{display:flex;flex-direction:column;gap:4px}
	.ha-pop-label{color:var(--color-text-subdue);font-size:11px}
	.ha-pop-current{color:var(--color-text-subdue);font-size:11px}
	.ha-pop-slider{width:100%;height:14px;margin:0;-webkit-appearance:none;appearance:none;background:transparent;cursor:pointer}
	.ha-pop-slider::-webkit-slider-runnable-track{height:4px;border-radius:2px;
	  background:linear-gradient(90deg,var(--color-primary) var(--pct,0%),color-mix(in srgb,var(--color-text-base) 16%,transparent) var(--pct,0%))}
	.ha-pop-slider::-moz-range-track{height:4px;border-radius:2px;background:color-mix(in srgb,var(--color-text-base) 16%,transparent)}
	.ha-pop-slider::-moz-range-progress{height:4px;border-radius:2px;background:var(--color-primary)}
	.ha-pop-slider::-webkit-slider-thumb{-webkit-appearance:none;width:14px;height:14px;border-radius:50%;background:var(--color-primary);margin-top:-5px;border:0}
	.ha-pop-slider::-moz-range-thumb{width:14px;height:14px;border-radius:50%;background:var(--color-primary);border:0}
	.ha-pop-slider-temp::-webkit-slider-runnable-track{background:linear-gradient(90deg,#ffcc80,#fff4e0,#cfe8ff)}
	.ha-pop-slider-temp::-moz-range-track{background:linear-gradient(90deg,#ffcc80,#fff4e0,#cfe8ff)}
	.ha-pop-swatches{display:flex;flex-wrap:wrap;gap:6px}
	.ha-pop-swatch{width:22px;height:22px;border-radius:50%;border:1px solid rgba(255,255,255,.25);cursor:pointer;padding:0}
`

// entityControlScript is a second onerror-driven IIFE, appended after
// bootstrapCore (see template.go), sharing nothing with it but the DOM: it
// re-derives root from the same img element, and relies on bootstrapCore's
// poll loop (via the data-hold-until guard below) to reconcile state rather
// than triggering its own.
//
// Click vs. long-press is decided with pointer events (unifies mouse and
// touch — this widget is as likely to run on a wall-mounted touchscreen as
// a desktop browser): a short tap toggles the entity; holding for
// longPressMs opens the popover for entities that have one (a light with
// brightness/color, or a climate device); movement past moveTolerance
// cancels both, so a scroll/swipe starting on a tile is never mistaken for
// a press.
const entityControlScript = `;(function(img){
	var root=img.closest('.ha-widget');if(!root)return;
	var entityUrl=root.dataset.entityUrl;if(!entityUrl)return;

	var TOGGLE_DOMAINS={light:true,switch:true,fan:true,cover:true,lock:true,humidifier:true,water_heater:true,vacuum:true,climate:true};

	function eligibleTile(el){
		var tile=el.closest&&el.closest('.ha-light,.ha-device');
		if(!tile)return null;
		if(tile.classList.contains('ha-device')&&!TOGGLE_DOMAINS[tile.dataset.domain])return null;
		return tile;
	}
	function canPopover(tile){
		if(tile.classList.contains('ha-light'))return tile.dataset.hasBrightness==='true'||tile.dataset.hasColorTemp==='true'||tile.dataset.hasColor==='true';
		return tile.dataset.hasTargetTemp==='true';
	}
	root.querySelectorAll('.ha-light,.ha-device').forEach(function(t){if(eligibleTile(t)===t)t.dataset.fpInteractive='true';});

	function post(action,entityId,data){
		var body=Object.assign({entity_id:entityId,action:action},data||{});
		return fetch(entityUrl,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'text/plain'},body:JSON.stringify(body)});
	}
	function toggleTile(tile){
		var willOn=tile.dataset.on!=='true';
		tile.dataset.on=willOn;
		post('toggle',tile.dataset.entityId).then(function(r){if(!r.ok)tile.dataset.on=String(!willOn);}).catch(function(){tile.dataset.on=String(!willOn);});
	}

	var pop=null,popFor=null;
	function onKey(e){if(e.key==='Escape')closePopover();}
	function onOutside(e){if(pop&&!pop.contains(e.target)&&e.target!==popFor&&!popFor.contains(e.target))closePopover();}
	function closePopover(){
		if(!pop)return;
		pop.remove();pop=null;popFor=null;
		document.removeEventListener('keydown',onKey);
		document.removeEventListener('pointerdown',onOutside,true);
	}

	var swatches=['#ff3b30','#ff9500','#ffcc00','#34c759','#30d5c8','#0a84ff','#af52de','#ff2d78'];
	function pct(v,min,max){return Math.max(0,Math.min(100,Math.round(100*(v-min)/((max-min)||1))));}

	function buildPanel(tile){
		var isLight=tile.classList.contains('ha-light');
		var panel=document.createElement('div');
		panel.className='ha-pop';
		var on=tile.dataset.on==='true';
		var html='<div class="ha-pop-head"><span class="ha-pop-title">'+(tile.title||(isLight?'Light':'Climate'))+
			'</span><button type="button" class="ha-pop-toggle" data-on="'+on+'" aria-label="Toggle"></button></div>';
		if(isLight){
			if(tile.dataset.hasBrightness==='true'){
				var b=+tile.dataset.brightness||0;
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Brightness</span>'+
					'<input type="range" class="ha-pop-slider" data-field="brightness" min="1" max="100" step="1" value="'+b+'" style="--pct:'+b+'%"></div>';
			}
			if(tile.dataset.hasColorTemp==='true'){
				var ctMin=+tile.dataset.colorTempMin||2000,ctMax=+tile.dataset.colorTempMax||6500;
				var ct=+tile.dataset.colorTemp||Math.round((ctMin+ctMax)/2);
				html+='<div class="ha-pop-row"><span class="ha-pop-label">Color temperature</span>'+
					'<input type="range" class="ha-pop-slider ha-pop-slider-temp" data-field="color_temp" min="'+ctMin+'" max="'+ctMax+'" step="50" value="'+ct+'" style="--pct:'+pct(ct,ctMin,ctMax)+'%"></div>';
			}
			if(tile.dataset.hasColor==='true'){
				html+='<div class="ha-pop-row ha-pop-swatches">'+swatches.map(function(c){return '<button type="button" class="ha-pop-swatch" data-rgb-hex="'+c+'" style="background:'+c+'" aria-label="'+c+'"></button>';}).join('')+'</div>';
			}
		}else{
			var min=+tile.dataset.minTemp||16,max=+tile.dataset.maxTemp||30,step=+tile.dataset.tempStep||0.5;
			var tgt=+tile.dataset.targetTemp||min,cur=tile.dataset.currentTemp;
			if(cur)html+='<div class="ha-pop-current">Currently '+(+cur).toFixed(1)+'°</div>';
			html+='<div class="ha-pop-row"><span class="ha-pop-label ha-pop-target-label">Target '+tgt.toFixed(1)+'°</span>'+
				'<input type="range" class="ha-pop-slider" data-field="temperature" min="'+min+'" max="'+max+'" step="'+step+'" value="'+tgt+'" style="--pct:'+pct(tgt,min,max)+'%"></div>';
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
		panel.style.top=y+'px';
	}

	var debounceTimers={};
	function scheduleSend(tile,field,value,immediate){
		tile.dataset.holdUntil=String(Date.now()+8000);
		var key=tile.dataset.entityId+':'+field;
		clearTimeout(debounceTimers[key]);
		var run=function(){
			var data={},action;
			if(field==='brightness'){data.brightness_pct=value;action='set_brightness';}
			else if(field==='color_temp'){data.color_temp_kelvin=value;action='set_color_temp';}
			else{data.temperature=value;action='set_temperature';}
			post(action,tile.dataset.entityId,data).catch(function(){});
		};
		if(immediate)run();else debounceTimers[key]=setTimeout(run,250);
	}

	function wirePanel(panel,tile){
		var toggleBtn=panel.querySelector('.ha-pop-toggle');
		if(toggleBtn)toggleBtn.addEventListener('click',function(){toggleTile(tile);toggleBtn.dataset.on=tile.dataset.on;});
		panel.querySelectorAll('.ha-pop-slider').forEach(function(input){
			input.addEventListener('input',function(){
				input.style.setProperty('--pct',pct(+input.value,+input.min,+input.max)+'%');
				if(input.dataset.field==='temperature'){
					var label=panel.querySelector('.ha-pop-target-label');
					if(label)label.textContent='Target '+(+input.value).toFixed(1)+'°';
				}
				scheduleSend(tile,input.dataset.field,+input.value,false);
			});
			input.addEventListener('change',function(){scheduleSend(tile,input.dataset.field,+input.value,true);});
		});
		panel.querySelectorAll('.ha-pop-swatch').forEach(function(btn){
			btn.addEventListener('click',function(){
				var hex=btn.dataset.rgbHex;
				var rgb=[parseInt(hex.slice(1,3),16),parseInt(hex.slice(3,5),16),parseInt(hex.slice(5,7),16)];
				tile.dataset.holdUntil=String(Date.now()+8000);
				post('set_color',tile.dataset.entityId,{rgb_color:rgb}).catch(function(){});
			});
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
	}

	var press=null,LONG_PRESS_MS=500,MOVE_TOLERANCE=10;
	root.addEventListener('pointerdown',function(e){
		var tile=eligibleTile(e.target);
		if(!tile)return;
		press={tile:tile,x:e.clientX,y:e.clientY};
		press.timer=setTimeout(function(){
			if(!press)return;
			var t=press.tile;press=null;
			if(canPopover(t))openPopover(t);
		},LONG_PRESS_MS);
	});
	root.addEventListener('pointermove',function(e){
		if(!press)return;
		if(Math.abs(e.clientX-press.x)>MOVE_TOLERANCE||Math.abs(e.clientY-press.y)>MOVE_TOLERANCE){clearTimeout(press.timer);press=null;}
	});
	root.addEventListener('pointerup',function(){
		if(!press)return;
		clearTimeout(press.timer);
		var tile=press.tile;press=null;
		toggleTile(tile);
	});
	root.addEventListener('pointercancel',function(){if(press){clearTimeout(press.timer);press=null;}});
})(this)`
