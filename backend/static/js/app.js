import {
    initThree,
    loadTextureParams,
    updateStatsPanel,
} from './plank_road_3d.js';

import {
    initClock,
    updateClock,
    loadDashboard,
    selectSite,
    setupEventListeners,
    onPanelResize,
} from './weathering_panel.js';

const state = {
    currentSiteId: 1,
    sites: [],
    currentSite: null,
    simulation: null,
    weathering: null,
    sensorData: [],
    alarms: [],
    showStress: true,
    showCracks: true,
    showSensors: true,
    wireframe: false,
    stressMin: 0,
    stressMax: 25,
    renderMode: 'stress',
    chartTab: 'strain',
    scene: null,
    camera: null,
    renderer: null,
    controls: null,
    plankRoadGroup: null,
    woodMaterial: null,
    rockMaterial: null,
    sensorMarkers: [],
    crackMeshes: [],
    textureCache: {},
};

async function init() {
    initClock();
    await loadTextureParams();
    initThree(state);
    setupEventListeners(state);
    await loadDashboard(state);

    setInterval(updateClock, 1000);
    setInterval(() => loadDashboard(state), 30000);

    window.addEventListener('resize', () => onPanelResize(state));

    setTimeout(() => {
        document.getElementById('loadingOverlay').classList.add('hidden');
    }, 1000);
}

init();
