// TideFlow v2

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
        const f = ref({ name: '', url: '' });
        const s = ref({});
        const sources = ref([]);
        const stats = ref({
            speed: 0, traffic: 0, traffic_cap: 0, cap_enabled: false,
            stream_count: 0, stream_bytes: {}, tasks: [], max_concurrent: 3,
            in_window: true, window_enabled: false,
            window_start: '', window_end: '',
            paused_cap: false, paused_window: false,
            cooldown_ids: [], all_failed: false,
        });
        const streamList = ref([]);
        const idleList = ref([]);
        let timer = null;

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
                const n = sources.value.length;  // 确保追踪 sources
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
                    idleList.push({ id: src.id, name: src.name, status: st });
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
        function addSource() { f.value = { name:'', url:'' }; testResult.value = null; showForm.value = true; }
        async function testUrl() {
            if (!f.value.url) return;
            testing.value = true; testResult.value = null;
            const r = await api('/api/sources/test', { method:'POST', body:JSON.stringify({url:f.value.url}) });
            testResult.value = r; testing.value = false;
        }
        async function saveSource() {
            if (!f.value.name || !f.value.url) return;
            await api('/api/sources', { method:'POST', body:JSON.stringify({...f.value, source_type:'http', enabled:true}) });
            showForm.value = false; f.value = { name:'', url:'' };
            await refresh();
        }
        function del(id) { const s = sources.value.find(x=>x.id===id); if(s) confirmDel.value = { id, name: s.name }; }
        async function doDel() { if(confirmDel.value) { await api('/api/sources/'+confirmDel.value.id, { method:'DELETE' }); confirmDel.value=null; refresh(); } }

        onMounted(async () => {
            await loadSettings();
            await refresh();
            timer = setInterval(refresh, 3000);
        });
        onUnmounted(() => { if(timer) clearInterval(timer); });

        return { mobileShow, showForm, testing, testResult, f, s, sources, stats, streamList, idleList,
                 dotClass, centerState, fmt, gb, save, act, addSource, testUrl, saveSource, del, confirmDel, doDel, unlocked, pwd, pwdErr, tryUnlock };
    }
}).mount('#app');
