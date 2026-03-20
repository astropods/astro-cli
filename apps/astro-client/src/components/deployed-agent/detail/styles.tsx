import { C } from "./theme";

export function Styles() {
  return (
    <style>{`
      @keyframes dp-pulse { 0%,100% { opacity:1; } 50% { opacity:0.4; } }
      @keyframes dp-blink { 0%,100% { opacity:1; } 50% { opacity:0; } }
      @keyframes dp-fadein { from { opacity:0; transform:translateY(3px); } to { opacity:1; transform:translateY(0); } }
      @keyframes dp-spin { from { transform:rotate(0deg); } to { transform:rotate(360deg); } }
      @keyframes dp-slot-in { 0% { transform:translateY(110%); opacity:0.5; } 65% { transform:translateY(-6%); opacity:1; } 82% { transform:translateY(2%); } 100% { transform:translateY(0); } }
      .dp-slot-in { animation: dp-slot-in 0.32s cubic-bezier(0.34,1.56,0.64,1) forwards; }
      .dp-blink { animation: dp-blink 1.1s step-end infinite; }
      .dp-pulse { animation: dp-pulse 1.8s ease-in-out infinite; }
      .dp-log { animation: dp-fadein 0.2s ease forwards; }
      .dp-spin { animation: dp-spin 1.2s linear infinite; }
      .dp-scroll { scrollbar-width: thin; scrollbar-color: transparent transparent; scrollbar-gutter: stable; }
      .dp-scroll:hover { scrollbar-color: #c4b89e transparent; }
      .dp-scroll::-webkit-scrollbar { width: 6px; }
      .dp-scroll::-webkit-scrollbar-track { background: transparent; }
      .dp-scroll::-webkit-scrollbar-thumb { background: transparent; border-radius: 3px; }
      .dp-scroll:hover::-webkit-scrollbar-thumb { background: #c4b89e; }
      .dp-container-hdr:hover { background: ${C.panel} !important; }
    `}</style>
  );
}
