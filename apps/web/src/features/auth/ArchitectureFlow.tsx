import {
  AudioWaveform,
  Clapperboard,
  Cpu,
  Database,
  Film,
  FileJson,
  Play,
  ScanSearch,
  Scissors,
  ServerCog
} from "lucide-react";
import cyclingSceneURL from "../../assets/login-cycling-scene.png";

function Waveform({ count = 22 }: { count?: number }) {
  return (
    <span className="architecture-waveform">
      {Array.from({ length: count }, (_, index) => <i key={index} />)}
    </span>
  );
}

export function ArchitectureFlow() {
  return (
    <div className="architecture-flow" aria-label="快剪辑系统处理架构">
      <div className="architecture-flow-head">
        <span>SYSTEM ARCHITECTURE / GENERATION FLOW</span>
        <span className="architecture-live"><i />PIPELINE ACTIVE</span>
      </div>

      <div className="architecture-stage" aria-hidden="true">
        <div className="architecture-grid" />
        <div className="architecture-route architecture-route-client"><span /></div>
        <div className="architecture-route architecture-route-output"><span /></div>

        <section className="architecture-layer architecture-client-layer">
          <header><span>01</span><strong>CLIENT</strong><small>LOCAL WORKSPACE</small></header>
          <div className="client-source-stack">
            <div className="source-frame source-frame-back"><img src={cyclingSceneURL} alt="" draggable={false} /></div>
            <div className="source-frame source-frame-middle"><img src={cyclingSceneURL} alt="" draggable={false} /></div>
            <div className="source-frame source-frame-front">
              <img src={cyclingSceneURL} alt="" draggable={false} />
              <span className="source-play"><Play size={14} fill="currentColor" /></span>
              <span className="source-trim source-trim-in">I</span>
              <span className="source-trim source-trim-out">O</span>
            </div>
          </div>
          <div className="client-agent-node">
            <span><Scissors size={17} /></span>
            <div><strong>Local Agent</strong><small>CLEAN SHOT / I·O</small></div>
          </div>
          <div className="client-analyzers">
            <span><ScanSearch size={13} />VLM</span>
            <span><AudioWaveform size={13} />ASR</span>
          </div>
        </section>

        <section className="architecture-layer architecture-core-layer">
          <header>
            <span>02</span><strong>AI CORE</strong><small>SERVER PROCESSING</small>
            <i className="core-health">ONLINE</i>
          </header>
          <div className="core-service-grid">
            <div><ServerCog size={16} /><span><strong>API</strong><small>GATEWAY</small></span></div>
            <div><Cpu size={16} /><span><strong>WORKER</strong><small>QUEUE</small></span></div>
            <div><Database size={16} /><span><strong>PGVECTOR</strong><small>RETRIEVAL</small></span></div>
          </div>
          <div className="core-processing-grid">
            <div className="core-semantic-block">
              <div className="core-block-title"><ScanSearch size={14} /><span>素材理解</span></div>
              <div className="semantic-tags"><span>产品特写</span><span>动作展示</span><span>固定机位</span></div>
              <div className="semantic-vector">{Array.from({ length: 18 }, (_, index) => <i key={index} />)}</div>
            </div>
            <div className="core-script-block">
              <div className="core-block-title"><FileJson size={14} /><span>文案 / 配音</span></div>
              <p>script_text</p>
              <Waveform />
            </div>
          </div>
          <div className="core-plan-strip">
            <span><FileJson size={13} />VISUAL BEATS</span>
            <i /><i /><i /><i />
            <strong>EDIT PLAN</strong>
          </div>
        </section>

        <section className="architecture-layer architecture-output-layer">
          <header><span>03</span><strong>OUTPUT</strong><small>RENDER PIPELINE</small></header>
          <div className="output-timeline">
            <div className="timeline-ruler"><span>00:00</span><span>00:08</span><span>00:16</span></div>
            <div className="timeline-video-track"><i /><i /><i /><i /></div>
            <Waveform count={18} />
            <span className="timeline-playhead" />
          </div>
          <div className="output-render-node"><Clapperboard size={15} /><span>FFMPEG RENDER</span><strong>76%</strong></div>
          <div className="output-preview">
            <div className="output-screen">
              <div className="output-scene"><img src={cyclingSceneURL} alt="" draggable={false} /><span className="output-subtitle">骑行更轻松</span></div>
            </div>
            <div className="output-meta"><span>9:16</span><strong>READY</strong></div>
          </div>
        </section>
      </div>

      <div className="architecture-flow-foot">
        <span><i className="is-client" />LOCAL AGENT</span>
        <span><i className="is-core" />MODEL GATEWAY</span>
        <span><i className="is-output" />RENDERER</span>
      </div>
    </div>
  );
}
