import { Link, useNavigate } from "react-router";
import { Paperclip, Mic, Send } from "lucide-react";

const quickActions = [
  "Surface customer insights",
  "Route incident alerts",
  "Draft support replies",
  "Scan for vulnerabilities",
  "Track sprint progress",
  "I'm not sure",
];

export default function Home() {
  const navigate = useNavigate();

  const handleQuickAction = () => {
    navigate("/hire?start=true");
  };

  return (
    <div className="flex items-center justify-center min-h-[calc(100vh-48px)]">
      <div className="max-w-[600px] w-full text-center">
        <h1 className="text-3xl font-semibold mb-2">Welcome to Astro</h1>
        <p className="text-stone-600 mb-6">What can I help you solve?</p>

        <div className="border border-stone-300 p-3 mb-4 text-left">
          <textarea
            className="w-full border-none bg-transparent text-sm resize-none focus:outline-none"
            placeholder="Describe the problem you're trying to solve..."
            rows={4}
          />
          <div className="flex justify-between items-center pt-2 border-t border-stone-300 mt-2">
            <button
              className="flex items-center justify-center w-8 h-8 border border-stone-300 bg-white cursor-pointer"
              title="Attach file"
            >
              <Paperclip size={18} />
            </button>
            <div className="flex gap-1">
              <button
                className="flex items-center justify-center w-8 h-8 border border-stone-300 bg-white cursor-pointer"
                title="Voice input"
              >
                <Mic size={18} />
              </button>
              <button
                className="flex items-center justify-center w-8 h-8 bg-stone-800 text-white border border-stone-800 cursor-pointer"
                title="Send"
              >
                <Send size={18} />
              </button>
            </div>
          </div>
        </div>

        <div className="flex flex-wrap gap-2 justify-center mb-6">
          {quickActions.map((action) => (
            <button
              key={action}
              className="px-3 py-1.5 border border-stone-300 bg-white text-sm cursor-pointer hover:bg-stone-50"
              onClick={handleQuickAction}
            >
              {action}
            </button>
          ))}
        </div>

        <Link to="/hire" className="text-sm text-stone-700 underline">
          Browse all agents
        </Link>
      </div>
    </div>
  );
}
