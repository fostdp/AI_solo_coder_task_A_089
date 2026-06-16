import * as THREE from 'three';
import {
    buildPlankRoad,
    applyStressColors,
    setWireframe,
    gradeColors,
} from './plank_road_3d.js';

const API_BASE = '/api';

export const gradeTextMap = {
    SLIGHT: '轻微',
    MILD: '轻度',
    MODERATE: '中度',
    SERIOUS: '严重',
    SEVERE: '极重'
};

export function initClock() {
    updateClock();
}

export function updateClock() {
    const now = new Date();
    document.getElementById('clock').textContent = now.toLocaleTimeString('zh-CN', { hour12: false });
}

export async function loadDashboard(state) {
    try {
        const res = await fetch(`${API_BASE}/dashboard`);
        const data = await res.json();
        if (data.code === 0) {
            renderSiteList(state, data.data.site_statuses);
            const conn = document.querySelector('.connection-status');
            conn.querySelector('.status-dot').classList.add('connected');
            conn.querySelector('span:last-child').textContent = '已连接';
        }
    } catch (e) {
        console.warn('Dashboard load failed:', e);
    }

    try {
        const alarmRes = await fetch(`${API_BASE}/alarms`);
        const alarmData = await alarmRes.json();
        if (alarmData.code === 0) {
            state.alarms = alarmData.data;
            renderAlarms(state);
        }
    } catch (e) {}

    if (state.currentSite) {
        loadSensorData(state, state.currentSiteId);
    }
}

export function renderSiteList(state, statuses) {
    state.sites = statuses;
    const container = document.getElementById('siteList');

    if (!statuses || statuses.length === 0) {
        container.innerHTML = '<div class="loading">暂无遗址数据</div>';
        return;
    }

    container.innerHTML = '';
    statuses.forEach((s) => {
        const div = document.createElement('div');
        div.className = 'site-item' + (s.site_id === state.currentSiteId ? ' active' : '');
        div.innerHTML = `
            <div class="site-item-header">
                <span class="site-name">${s.site_name}</span>
                <span class="status-badge status-${s.status.toLowerCase()}">${s.status}</span>
            </div>
            <div class="site-region">${s.region} · 遗址ID:${s.site_id}</div>
            <div class="site-meta">
                <span class="site-stat">🔔 ${s.alarm_count}</span>
                ${s.safety_factor > 0 ? `<span class="site-stat">🛡️ ${s.safety_factor.toFixed(2)}</span>` : ''}
                ${s.predicted_lifespan > 0 ? `<span class="site-stat">⏳ ${s.predicted_lifespan.toFixed(0)}年</span>` : ''}
            </div>
        `;
        div.addEventListener('click', () => selectSite(state, s.site_id));
        container.appendChild(div);
    });
}

export async function selectSite(state, siteId) {
    state.currentSiteId = siteId;
    document.querySelectorAll('.site-item').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.site-item').forEach((el, i) => {
        if (state.sites[i]?.site_id === siteId) el.classList.add('active');
    });

    try {
        const res = await fetch(`${API_BASE}/sites/${siteId}`);
        const data = await res.json();
        if (data.code === 0) {
            state.currentSite = data.data;
            renderSiteInfo(data.data);
            buildPlankRoad(state, data.data);
        }
    } catch (e) {}

    try {
        const simRes = await fetch(`${API_BASE}/sites/${siteId}/simulation`);
        const simData = await simRes.json();
        if (simData.code === 0) state.simulation = simData.data;
    } catch (e) {}

    try {
        const wRes = await fetch(`${API_BASE}/sites/${siteId}/weathering`);
        const wData = await wRes.json();
        if (wData.code === 0) state.weathering = wData.data;
        renderWeathering(state.weathering);
    } catch (e) {}

    applyStressColors(state);
    loadSensorData(state, siteId);
}

export function renderSiteInfo(site) {
    document.getElementById('siteInfoPanel').style.display = 'block';
    const info = document.getElementById('siteInfo');
    info.innerHTML = `
        <div class="info-item"><span class="info-label">名称</span><span class="info-value">${site.site_name}</span></div>
        <div class="info-item"><span class="info-label">地区</span><span class="info-value">${site.region}</span></div>
        <div class="info-item"><span class="info-label">海拔</span><span class="info-value">${site.elevation} m</span></div>
        <div class="info-item"><span class="info-label">年代</span><span class="info-value">${site.construction_era}</span></div>
        <div class="info-item"><span class="info-label">长度</span><span class="info-value">${site.total_length} m</span></div>
        <div class="info-item"><span class="info-label">梁数</span><span class="info-value">${site.beam_count}</span></div>
        <div class="info-item"><span class="info-label">岩体</span><span class="info-value">${site.rock_type}</span></div>
        <div class="info-item"><span class="info-label">木材</span><span class="info-value">${site.wood_type}</span></div>
    `;
}

export function renderWeathering(w) {
    document.getElementById('weatheringPanel').style.display = 'block';
    const el = document.getElementById('weatheringInfo');
    if (!w) {
        el.innerHTML = '<div class="loading">暂无风化评估数据，请点击"风化评估"按钮</div>';
        return;
    }

    const grade = w.weathering_grade || 'MODERATE';
    const gradeText = gradeTextMap[grade] || '中度';
    const lifePercent = w.predicted_lifespan ? Math.min(100, (w.remaining_lifespan / w.predicted_lifespan) * 100) : 50;
    const lifeColor = lifePercent < 20 ? 'var(--accent-red)' : lifePercent < 50 ? 'var(--accent-yellow)' : 'var(--accent-green)';

    el.innerHTML = `
        <div class="weathering-grade grade-${grade}">风化等级: ${gradeText}</div>
        <div class="weathering-detail">
            <div class="detail-row"><span class="detail-label">冻融循环</span><span class="detail-value">${w.freeze_thaw_cycles} 次</span></div>
            <div class="detail-row"><span class="detail-label">当前裂隙深</span><span class="detail-value">${w.current_crack_depth?.toFixed(2) || '--'} mm</span></div>
            <div class="detail-row"><span class="detail-label">年扩展率</span><span class="detail-value">${w.crack_propagation_rate?.toFixed(4) || '--'} mm/年</span></div>
            <div class="detail-row"><span class="detail-label">木材腐朽率</span><span class="detail-value">${w.wood_decay_rate?.toFixed(4) || '--'} %/年</span></div>
            <div class="detail-row"><span class="detail-label">岩体侵蚀率</span><span class="detail-value">${w.rock_erosion_rate?.toFixed(4) || '--'} mm/年</span></div>
            <div class="detail-row"><span class="detail-label">预测寿命</span><span class="detail-value">${w.predicted_lifespan?.toFixed(1) || '--'} 年</span></div>
            <div class="detail-row"><span class="detail-label">剩余寿命</span><span class="detail-value" style="color:${lifeColor}">${w.remaining_lifespan?.toFixed(1) || '--'} 年</span></div>
            <div class="detail-row"><span class="detail-label">置信度</span><span class="detail-value">${(w.confidence*100)?.toFixed(0) || '--'}%</span></div>
        </div>
        <div class="lifespan-bar">
            <div class="lifespan-fill" style="width:${lifePercent}%; background:${lifeColor}"></div>
        </div>
    `;
}

export async function loadSensorData(state, siteId) {
    try {
        const res = await fetch(`${API_BASE}/sensor?site_id=${siteId}&hours=72&limit=1000`);
        const data = await res.json();
        if (data.code === 0) {
            state.sensorData = data.data;
            drawChart(state);
        }
    } catch (e) {}
}

export function renderAlarms(state) {
    const container = document.getElementById('alarmList');
    if (!state.alarms || state.alarms.length === 0) {
        container.innerHTML = '<div class="no-alarm">✅ 系统运行正常，暂无告警</div>';
        return;
    }

    const siteNames = {};
    state.sites.forEach(s => siteNames[s.site_id] = s.site_name);

    container.innerHTML = '';
    state.alarms.slice(0, 50).forEach(a => {
        const div = document.createElement('div');
        div.className = `alarm-item level-${a.alarm_level}`;
        div.innerHTML = `
            <div class="alarm-header">
                <span class="alarm-type ${a.alarm_type}">${a.alarm_type === 'STRAIN' ? '梁孔应变' : '岩体裂隙'}</span>
                <span class="alarm-level">${a.alarm_level === 'WARNING' ? '警告' : '严重'}</span>
            </div>
            <div class="alarm-site">📍 ${siteNames[a.site_id] || '遗址#'+a.site_id} ${a.beam_id ? `· 梁#${a.beam_id}` : ''}</div>
            <div class="alarm-desc">${a.description}</div>
            <div class="alarm-time">⏰ ${new Date(a.time).toLocaleString('zh-CN')}</div>
        `;
        container.appendChild(div);
    });
}

export function drawChart(state) {
    const canvas = document.getElementById('chartCanvas');
    const container = canvas.parentElement;
    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;
    const ctx = canvas.getContext('2d');

    const data = state.sensorData || [];
    if (data.length === 0) {
        ctx.fillStyle = '#6c7a89';
        ctx.font = '14px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText('暂无监测数据', canvas.width / 2, canvas.height / 2);
        return;
    }

    const sorted = [...data].sort((a, b) => new Date(a.time) - new Date(b.time));
    const W = canvas.width, H = canvas.height;
    const pad = { l: 50, r: 20, t: 20, b: 40 };
    const cw = W - pad.l - pad.r;
    const ch = H - pad.t - pad.b;

    let series;
    const tab = state.chartTab;

    if (tab === 'strain') {
        series = [
            { label: '顶部应变 (με)', color: '#8b5cf6', key: 'beam_strain_top' },
            { label: '底部应变 (με)', color: '#06b6d4', key: 'beam_strain_bottom' },
            { label: '侧部应变 (με)', color: '#f59e0b', key: 'beam_strain_side' },
        ];
    } else if (tab === 'crack') {
        series = [
            { label: '裂隙1 (mm)', color: '#ef4444', key: 'rock_crack_width_1' },
            { label: '裂隙2 (mm)', color: '#f97316', key: 'rock_crack_width_2' },
            { label: '裂隙3 (mm)', color: '#f59e0b', key: 'rock_crack_width_3' },
        ];
    } else {
        series = [
            { label: '温度 (°C)', color: '#ef4444', key: 'temperature' },
            { label: '湿度 (%)', color: '#3b82f6', key: 'humidity', scale: 0.4 },
        ];
    }

    let minVal = Infinity, maxVal = -Infinity;
    series.forEach(s => {
        sorted.forEach(d => {
            const v = d[s.key] * (s.scale || 1);
            if (!isNaN(v)) { minVal = Math.min(minVal, v); maxVal = Math.max(maxVal, v); }
        });
    });
    const range = maxVal - minVal || 1;
    minVal -= range * 0.1;
    maxVal += range * 0.1;

    ctx.clearRect(0, 0, W, H);
    ctx.strokeStyle = 'rgba(45, 69, 96, 0.5)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 5; i++) {
        const y = pad.t + ch * i / 5;
        ctx.beginPath();
        ctx.moveTo(pad.l, y);
        ctx.lineTo(pad.l + cw, y);
        ctx.stroke();

        const val = maxVal - (maxVal - minVal) * i / 5;
        ctx.fillStyle = '#8fa3b8';
        ctx.font = '10px Courier New';
        ctx.textAlign = 'right';
        ctx.fillText(val.toFixed(1), pad.l - 6, y + 3);
    }

    const n = sorted.length;
    for (let i = 0; i < n; i += Math.max(1, Math.floor(n / 6))) {
        const x = pad.l + cw * i / (n - 1 || 1);
        ctx.beginPath();
        ctx.moveTo(x, pad.t);
        ctx.lineTo(x, pad.t + ch);
        ctx.stroke();

        const t = new Date(sorted[i].time);
        ctx.fillStyle = '#8fa3b8';
        ctx.textAlign = 'center';
        ctx.fillText(t.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }), x, pad.t + ch + 16);
    }

    series.forEach(s => {
        ctx.strokeStyle = s.color;
        ctx.lineWidth = 2;
        ctx.beginPath();
        let started = false;
        sorted.forEach((d, i) => {
            const v = d[s.key] * (s.scale || 1);
            if (isNaN(v)) return;
            const x = pad.l + cw * i / (n - 1 || 1);
            const y = pad.t + ch * (1 - (v - minVal) / (maxVal - minVal));
            if (!started) { ctx.moveTo(x, y); started = true; }
            else ctx.lineTo(x, y);
        });
        ctx.stroke();

        ctx.fillStyle = s.color;
        sorted.forEach((d, i) => {
            const v = d[s.key] * (s.scale || 1);
            if (isNaN(v)) return;
            const x = pad.l + cw * i / (n - 1 || 1);
            const y = pad.t + ch * (1 - (v - minVal) / (maxVal - minVal));
            ctx.beginPath();
            ctx.arc(x, y, 2.5, 0, Math.PI * 2);
            ctx.fill();
        });
    });

    ctx.font = '11px sans-serif';
    ctx.textAlign = 'left';
    series.forEach((s, i) => {
        const x = pad.l + 10 + i * 160;
        const y = pad.t + ch + 32;
        ctx.fillStyle = s.color;
        ctx.fillRect(x, y, 12, 3);
        ctx.fillStyle = '#e8edf2';
        ctx.fillText(s.label, x + 18, y + 4);
    });
}

export function setupEventListeners(state) {
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.chartTab = btn.dataset.tab;
            drawChart(state);
        });
    });

    document.getElementById('showStress').addEventListener('change', (e) => {
        state.showStress = e.target.checked;
        applyStressColors(state);
    });

    document.getElementById('showCracks').addEventListener('change', (e) => {
        state.showCracks = e.target.checked;
        if (state.currentSite) buildPlankRoad(state, state.currentSite);
    });

    document.getElementById('showSensors').addEventListener('change', (e) => {
        state.showSensors = e.target.checked;
        if (state.currentSite) buildPlankRoad(state, state.currentSite);
    });

    document.getElementById('showWireframe').addEventListener('change', (e) => {
        setWireframe(state, e.target.checked);
    });

    document.getElementById('stressMin').addEventListener('input', (e) => {
        state.stressMin = parseFloat(e.target.value);
        document.getElementById('stressMinVal').textContent = state.stressMin;
        applyStressColors(state);
    });

    document.getElementById('stressMax').addEventListener('input', (e) => {
        state.stressMax = parseFloat(e.target.value);
        document.getElementById('stressMaxVal').textContent = state.stressMax;
        applyStressColors(state);
    });

    document.getElementById('renderMode').addEventListener('change', (e) => {
        state.renderMode = e.target.value;
        applyStressColors(state);
    });

    document.getElementById('btnRefreshData').addEventListener('click', () => {
        loadSensorData(state, state.currentSiteId);
        loadDashboard(state);
        showToast('数据已刷新', 'success');
    });

    document.getElementById('btnSimulate').addEventListener('click', async () => {
        if (!state.currentSiteId) return showToast('请先选择遗址', 'warning');
        showToast('正在执行有限元仿真...', 'info');
        try {
            const res = await fetch(`${API_BASE}/sites/${state.currentSiteId}/simulate`, { method: 'POST' });
            const data = await res.json();
            if (data.code === 0) {
                state.simulation = data.data;
                applyStressColors(state);
                showToast(`仿真完成! 安全系数=${data.data.safety_factor.toFixed(2)}`, 'success');
            }
        } catch (e) {
            showToast('仿真失败: ' + e.message, 'error');
        }
    });

    document.getElementById('btnWeathering').addEventListener('click', async () => {
        if (!state.currentSiteId) return showToast('请先选择遗址', 'warning');
        showToast('正在执行风化评估...', 'info');
        try {
            const res = await fetch(`${API_BASE}/sites/${state.currentSiteId}/weathering`, { method: 'POST' });
            const data = await res.json();
            if (data.code === 0) {
                state.weathering = data.data;
                renderWeathering(data.data);
                buildPlankRoad(state, state.currentSite);
                const grade = gradeTextMap[data.data.weathering_grade] || '-';
                showToast(`评估完成! 等级=${grade} 剩余=${data.data.remaining_lifespan.toFixed(0)}年`, 'success');
            }
        } catch (e) {
            showToast('评估失败: ' + e.message, 'error');
        }
    });
}

export function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = { success: '✅', warning: '⚠️', error: '❌', info: 'ℹ️' }[type] || 'ℹ️';
    toast.innerHTML = `<span class="toast-icon">${icon}</span><span>${message}</span>`;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('closing');
        setTimeout(() => toast.remove(), 300);
    }, 4000);
}

export function onPanelResize(state) {
    drawChart(state);
}
