import { Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";

export function RequestAgent() {
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Form submission logic would go here
  };

  return (
    <div className="max-w-[600px]">
      <Link
        to="/hire"
        className="inline-flex items-center gap-1 text-sm text-gray-700 mb-6"
      >
        <ArrowLeft size={16} />
        Back to Agent Directory
      </Link>

      <div>
        <h1 className="text-2xl font-semibold mb-2">Request a Custom Agent</h1>
        <p className="text-gray-600 text-sm mb-6">
          Tell us about the problem you're trying to solve, and we'll help you
          build a custom agent tailored to your needs.
        </p>

        <form
          className="border border-gray-300 p-6"
          onSubmit={handleSubmit}
        >
          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="name" className="text-sm text-gray-600">
              Your Name *
            </label>
            <input
              type="text"
              id="name"
              placeholder="Enter your name"
              required
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="company" className="text-sm text-gray-600">
              Company Name *
            </label>
            <input
              type="text"
              id="company"
              placeholder="Enter your company name"
              required
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="email" className="text-sm text-gray-600">
              Work Email *
            </label>
            <input
              type="email"
              id="email"
              placeholder="you@company.com"
              required
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="problem" className="text-sm text-gray-600">
              What problem are you trying to solve? *
            </label>
            <textarea
              id="problem"
              placeholder="Describe the tasks or workflows you'd like to automate..."
              rows={5}
              required
              className="px-3 py-2 border border-gray-300 text-sm resize-y min-h-[80px] focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="systems" className="text-sm text-gray-600">
              What systems or apps do you use?
            </label>
            <input
              type="text"
              id="systems"
              placeholder="e.g., Slack, Salesforce, Notion, Jira..."
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="additional" className="text-sm text-gray-600">
              Anything else we should know?
            </label>
            <textarea
              id="additional"
              placeholder="Additional context, timeline, or specific requirements..."
              rows={4}
              className="px-3 py-2 border border-gray-300 text-sm resize-y min-h-[80px] focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <button
            type="submit"
            className="w-full px-4 py-2 bg-gray-800 text-white border border-gray-800 text-sm cursor-pointer hover:bg-gray-700"
          >
            Submit Request
          </button>
        </form>
      </div>
    </div>
  );
}
