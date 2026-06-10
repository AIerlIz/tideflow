// TideFlow v3

const { createApp, ref, computed, onMounted, onUnmounted } = Vue;
createApp({
	setup() {
		const mobileShow = ref(false);
		const unlocked = ref(false);
		const pwd = ref('');
		const pwdErr = ref(false);
		const showForm = ref(false);
		const confirmDel = ref(null);
		const testing = ref(false);
		const testResult = ref(null);
		const darkOn = ref(localStorage.getItem('tideflow-dark')!=='0');
		const f = ref({ name: '', url: '', max_speed: '', header_kv: '' });
		const s = ref({});
		const sources = ref([]);
		const history = ref([]);
		const stats = ref({
			speed: 0, traffic: 0, traffic_cap: 0, cap_enabled: false,
			stream_count: 0, stream_bytes: {}, tasks: [], max_concurrent: 3,
			in_window: true, window_enabled: false,
			window_start: '', window_end: '',
			paused_cap: false, paused_window: false,
			cooldown_ids: [], all_failed: false, all_time_bytes: 0,
		});
		const streamList = ref([]);
		const idleList = ref([]);
		let timer = null;

		// Dark mode
		function applyDark() {
			document.body.classList.toggle('light', !darkOn.value);
			localStorage.setItem('tideflow-dark', darkOn.value ? '1' : '0');
		}
		function toggleDark() { darkOn.value = !darkOn.value; applyDark(); }
		applyDark();

		const dotClass = computed(() => {
			const d = stats.value;
			if (d.paused_window) return 'warn';
			if (d.paused_cap || d.all_failed) return 'err';
			if (d.stream_count > 0) return 'on';
			return 'off';
		});

		const centerState = computed(() => {
			try {
				const d = stats.value;
				const n = sources.value.length;
				const p = d.paused_cap;
				const w = d.paused_window;
				const f = d.all_failed;
				if (n === 0) {
					return { icon: '🌊', title: '还没有下载源', desc: '在左侧面板添加下载源以开始', hint: '首次启动已自动添加默认源' };
				}
				if (p && d.traffic_cap > 0) {
					const pct = (d.traffic / d.traffic_cap * 100).toFixed(1);
					return { icon: '🛑', title: '已达流量上限', desc: `周期流量 <b style="color:#a78bfa">${fmt(d.traffic)}</b> / ${fmt(d.traffic_cap)}（${pct}%）`, hint: '下个周期自动重置恢复' };
				}
				if (w) {
					return { icon: '⏸', title: '不在下载时段内', desc: `时段 ${d.window_start} → ${d.window_end}`, hint: '进入时段后自动恢复' };
				}
				if (f) {
					return { icon: '⚠️', title: '所有下载源均失败', desc: '请检查源 URL 或点击「清除冷却」重试', hint: null };
				}
				return null;
			} catch(e) { return null; }
		});

		function fmt(b) {
			if (!b || b===0) return '0 B';
			const u = ['B','KB','MB','GB','TB'];
			const i = Math.floor(Math.log(b)/Math.log(1024));
			return (b/Math.pow(1024,i)).toFixed(i===0?0:1)+' '+u[i];
		}
		function gb(s) { try { return (parseInt(s)/1073741824).toFixed(1); } catch { return '0'; } }
		const chartTip = ref(-1);

		const chartLine = computed(() => {
			if (!history.value.length) return '';
			const vals = history.value.map(h => h.bytes);
			const max = Math.max(...vals, 1);
			const min = Math.min(...vals);
			const range = max - min || 1;
			return history.value.map((h, i) => {
				const x = i * 20 + 10;
				const y = 100 - ((h.bytes - min) / range) * 80 - 10;
				return `${x},${Math.round(y)}`;
			}).join(' ');
		});

		const chartArea = computed(() => {
			const pts = chartLine.value;
			if (!pts) return '';
			const last = history.value.length * 20 + 10;
			return `M 10,100 L ${pts} L ${last},100 Z`;
		});

		const chartPoints = computed(() => {
			if (!history.value.length) return [];
			const vals = history.value.map(h => h.bytes);
			const max = Math.max(...vals, 1);
			const min = Math.min(...vals);
			const range = max - min || 1;
			return history.value.map((h, i) => ({
				x: i * 20 + 10,
				y: Math.round(100 - ((h.bytes - min) / range) * 80 - 10),
			}));
		});

		async function api(url, opts={}) {
			try {
				const r = await fetch(url, { headers: {'Content-Type':'application/json'}, ...opts });
				return r.ok ? r.json() : null;
			} catch(e) { return null; }
		}

		function srcName(sid) {
			const src = sources.value.find(x => x.id === sid);
			return src ? src.name : '源#'+sid;
		}

		function parseHeaders(kv) {
			if (!kv || !kv.trim()) return {};
			const h = {};
			kv.split(',').forEach(p => {
				const idx = p.indexOf(':');
				if (idx > 0) h[p.slice(0,idx).trim()] = p.slice(idx+1).trim();
			});
			return h;
		}

		function updateLists() {
			const d = stats.value;
			const tasks = d.tasks || [];
			streamList.value = tasks.map(t => ({
				id: t.key, name: srcName(t.source_id), bytes: t.bytes, sid: t.source_id
			}));
			const cd = d.cooldown_ids || [];
			const busyIds = new Set(tasks.map(t => t.source_id));
			idleList.value = [];
			for (const src of sources.value) {
				if (!busyIds.has(src.id)) {
					let st = '等待';
					if (!src.enabled) st = '禁用';
					else if (cd.includes(src.id)) st = '冷却中';
					idleList.value.push({ id: src.id, name: src.name, status: st });
				}
			}
		}

		async function refresh() {
			const [st, src] = await Promise.all([api('/api/stats'), api('/api/sources')]);
			if (st) stats.value = st;
			if (src) sources.value = src;
			updateLists();
		}

		async function loadSettings() {
			const d = await api('/api/settings');
			if (d?.settings) s.value = d.settings;
		}
		async function loadHistory() {
			const d = await api('/api/stats/traffic?days=30');
			if (d && Array.isArray(d)) history.value = d.slice(-30);
		}
		async function save() { await api('/api/settings', { method:'PUT', body:JSON.stringify({settings:{...s.value}}) }); }

		async function act(type) {
			if (type === 'pause') await api('/api/downloads/pause', { method:'POST' });
			else if (type === 'resume') await api('/api/downloads/resume', { method:'POST' });
			else if (type === 'clear') await api('/api/sources/clear-cooldowns', { method:'POST' });
			refresh();
		}

		async function tryUnlock() {
			const r = await api('/api/auth', { method:'POST', body:JSON.stringify({password:pwd.value}) });
			if (r?.ok) { unlocked.value = true; pwdErr.value = false; }
			else { pwdErr.value = true; pwd.value = ''; }
		}
		function addSource() { f.value = { name:'', url:'', max_speed:'', header_kv:'' }; testResult.value = null; showForm.value = true; }
		async function testUrl() {
			if (!f.value.url) return;
			testing.value = true; testResult.value = null;
			const r = await api('/api/sources/test', { method:'POST', body:JSON.stringify({url:f.value.url}) });
			testResult.value = r; testing.value = false;
		}
		async function saveSource() {
			if (!f.value.name || !f.value.url) return;
			const headers = parseHeaders(f.value.header_kv);
			await api('/api/sources', { method:'POST', body:JSON.stringify({
				name: f.value.name, url: f.value.url, source_type: 'http', enabled: true,
				max_speed: f.value.max_speed || '', headers: headers
			})});
			showForm.value = false; f.value = { name:'', url:'', max_speed:'', header_kv:'' };
			await refresh();
		}
		function del(id) { const s = sources.value.find(x=>x.id===id); if(s) confirmDel.value = { id, name: s.name }; }
		async function doDel() { if(confirmDel.value) { await api('/api/sources/'+confirmDel.value.id, { method:'DELETE' }); confirmDel.value=null; refresh(); } }

		onMounted(async () => {
			await loadSettings();
			await loadHistory();
			await refresh();
			timer = setInterval(refresh, 3000);
		});
		onUnmounted(() => { if(timer) clearInterval(timer); });

		return { mobileShow, showForm, testing, testResult, f, s, sources, stats, streamList, idleList, history,
				 dotClass, centerState, fmt, gb, chartLine, chartArea, chartPoints, chartTip,
				 save, act, addSource, testUrl, saveSource, del, confirmDel, doDel,
				 unlocked, pwd, pwdErr, tryUnlock, darkOn, toggleDark };
	}
}).mount('#app');
