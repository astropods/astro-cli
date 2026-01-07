import { useState, useEffect, useRef, useCallback } from "react";
import { Handle, Position, useUpdateNodeInternals, type NodeProps, type Node } from "@xyflow/react";
import { codeToHtml } from "shiki";
import { Play, Braces, Sparkles, GitFork, type LucideIcon } from "lucide-react";
import type { AgentNodeData } from "../types";

const nodeTypeConfig: Record<string, { container: string; title: string; icon: LucideIcon }> = {
  start: {
    container: "bg-[#0d2818] border-emerald-500/50",
    title: "text-emerald-200",
    icon: Play,
  },
  evaluate: {
    container: "bg-[#2a1f0a] border-amber-500/50",
    title: "text-amber-200",
    icon: Braces,
  },
  generate: {
    container: "bg-[#1a0f2e] border-violet-500/50",
    title: "text-violet-200",
    icon: Sparkles,
  },
  if: {
    container: "bg-[#0a1929] border-sky-500/50",
    title: "text-sky-200",
    icon: GitFork,
  },
};

type AgentNodeType = Node<AgentNodeData, "agent">;

// Extract and format function code from the fn property
function extractFunctionCode(fn: unknown): string | null {
  if (typeof fn !== "function") return null;
  
  const fnString = fn.toString();
  
  // Try to clean up the function string for better readability
  // Remove leading/trailing whitespace and normalize indentation
  const lines = fnString.split("\n");
  
  // Find minimum indentation (excluding empty lines)
  const minIndent = lines
    .filter((line) => line.trim().length > 0)
    .reduce((min, line) => {
      const match = line.match(/^(\s*)/);
      const indent = match ? match[1].length : 0;
      return Math.min(min, indent);
    }, Infinity);
  
  // Remove the common indentation
  const normalized = lines
    .map((line) => line.slice(minIndent === Infinity ? 0 : minIndent))
    .join("\n")
    .trim();
  
  return normalized;
}

interface CodeAccordionProps {
  code: string;
  onToggle?: (isOpen: boolean) => void;
}

function CodeAccordion({ code, onToggle }: CodeAccordionProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [highlightedHtml, setHighlightedHtml] = useState<string>("");
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen && !highlightedHtml) {
      codeToHtml(code, {
        lang: "javascript",
        theme: "vitesse-dark",
      }).then(setHighlightedHtml);
    }
  }, [isOpen, code, highlightedHtml]);

  const handleToggle = useCallback(() => {
    const newIsOpen = !isOpen;
    setIsOpen(newIsOpen);
    // Notify parent after a brief delay to allow CSS transition to complete
    if (onToggle) {
      setTimeout(() => onToggle(newIsOpen), 220);
    }
  }, [isOpen, onToggle]);

  return (
    <div className="border-t border-white/10">
      <button
        onClick={handleToggle}
        className="w-full px-3 py-2 flex items-center gap-2 text-[11px] font-medium text-amber-300/70 hover:text-amber-300 hover:bg-white/5 transition-colors"
      >
        <svg
          className={`w-3 h-3 transition-transform duration-200 ${isOpen ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span>Show Code</span>
      </button>
      
      <div
        ref={contentRef}
        className={`overflow-hidden transition-all duration-200 ease-out ${
          isOpen ? "max-h-[300px]" : "max-h-0"
        }`}
      >
        <div className="px-3 py-2 overflow-auto max-h-[280px] bg-zinc-950 rounded-b-xl nowheel">
          {highlightedHtml ? (
            <div
              className="text-[11px] leading-relaxed [&_pre]:bg-transparent! [&_pre]:p-0! [&_pre]:m-0! [&_code]:bg-transparent!"
              dangerouslySetInnerHTML={{ __html: highlightedHtml }}
            />
          ) : (
            <pre className="text-[11px] leading-relaxed text-zinc-400 font-mono whitespace-pre-wrap">
              {code}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
}

export function AgentNode({ id, data }: NodeProps<AgentNodeType>) {
  const updateNodeInternals = useUpdateNodeInternals();
  const inputPorts = data.ports.filter((p) => p.type === "input");
  const outputPorts = data.ports.filter((p) => p.type === "output");

  const config = nodeTypeConfig[data.nodeType] || {
    container: "bg-[#1f1f23] border-zinc-500/50",
    title: "text-zinc-200",
    icon: Braces,
  };
  
  const Icon = config.icon;

  // Extract function code for evaluate nodes
  const functionCode =
    data.nodeType === "evaluate" && data.nodeData?.fn
      ? extractFunctionCode(data.nodeData.fn)
      : null;

  // Handle accordion toggle - notify React Flow that node internals changed
  const handleAccordionToggle = useCallback(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals]);

  return (
    <div
      className={`border rounded-xl shadow-xl shadow-black/20 min-w-[180px] max-w-[320px] ${config.container}`}
    >
      {/* Header */}
      <div className="px-4 py-3 border-b border-white/10">
        <div className={`flex items-center justify-center gap-2 font-medium text-sm ${config.title}`}>
          <Icon className="w-4 h-4" />
          <span>{data.label}</span>
        </div>
      </div>

      {/* Input handles */}
      {inputPorts.map((port, i) => (
        <Handle
          key={port.id}
          type="target"
          position={Position.Top}
          id={port.id}
          className="w-3! h-3! bg-zinc-600! border-2! border-zinc-400! -top-1.5!"
          style={{
            left: `${((i + 1) / (inputPorts.length + 1)) * 100}%`,
          }}
        />
      ))}

      {/* Output ports section */}
      {outputPorts.length > 1 && (
        <div className="px-4 py-2 flex justify-around gap-2">
          {outputPorts.map((port) => (
            <span
              key={port.id}
              className="text-[10px] text-zinc-500 font-medium"
            >
              {port.id}
            </span>
          ))}
        </div>
      )}

      {/* Code accordion for evaluate nodes */}
      {functionCode && <CodeAccordion code={functionCode} onToggle={handleAccordionToggle} />}

      {/* Output handles */}
      {outputPorts.map((port, i) => (
        <Handle
          key={port.id}
          type="source"
          position={Position.Bottom}
          id={port.id}
          className="w-3! h-3! bg-zinc-600! border-2! border-zinc-400! -bottom-1.5!"
          style={{
            left: `${((i + 1) / (outputPorts.length + 1)) * 100}%`,
          }}
        />
      ))}
    </div>
  );
}
