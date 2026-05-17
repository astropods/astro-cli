import { useState } from "react";
import { LibraryView } from "./components/LibraryView";
import { SourceView } from "./components/SourceView";

type Tab = "library" | "source";

export function App() {
  const [tab, setTab] = useState<Tab>("library");
  const [refreshKey, setRefreshKey] = useState(0);

  return (
    <div className="flex flex-col h-full">
      <header className="border-b border-white/10 px-6 py-4 flex items-center gap-6 bg-[#0b0c0f]">
        <div className="font-mono text-sm font-medium tracking-tight">
          brand-icons
        </div>
        <nav className="flex gap-1 text-sm">
          <TabButton active={tab === "library"} onClick={() => setTab("library")}>
            Library
          </TabButton>
          <TabButton active={tab === "source"} onClick={() => setTab("source")}>
            Source new
          </TabButton>
        </nav>
        <div className="ml-auto text-xs text-white/40 font-mono">
          @astropods/brand-icons
        </div>
      </header>
      <main className="flex-1 min-h-0 flex flex-col">
        <div
          className={
            "flex-1 overflow-auto " + (tab === "library" ? "" : "hidden")
          }
        >
          <LibraryView
            refreshKey={refreshKey}
            onRebuilt={() => setRefreshKey((k) => k + 1)}
          />
        </div>
        <div
          className={
            "flex-1 min-h-0 flex flex-col " + (tab === "source" ? "" : "hidden")
          }
        >
          <SourceView onLibraryChanged={() => setRefreshKey((k) => k + 1)} />
        </div>
      </main>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "px-3 py-1.5 rounded-md transition-colors " +
        (active
          ? "bg-white/10 text-white"
          : "text-white/60 hover:text-white hover:bg-white/5")
      }
    >
      {children}
    </button>
  );
}
