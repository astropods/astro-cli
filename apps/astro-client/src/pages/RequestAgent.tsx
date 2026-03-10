import { Link } from "react-router";
import { ArrowLeft } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";

export default function RequestAgent() {
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Form submission logic would go here
  };

  return (
    <div className="max-w-[600px] p-6 md:p-8">
      <Link
        to="/browse"
        className="inline-flex items-center gap-1 text-sm text-stone-700 mb-6"
      >
        <ArrowLeft size={16} />
        Back to Agent Directory
      </Link>

      <div>
        <h1 className="text-heading-1 mb-2">Request a Custom Agent</h1>
        <p className="text-stone-600 text-sm mb-6">
          Tell us about the problem you're trying to solve, and we'll help you
          build a custom agent tailored to your needs.
        </p>

        <form
          className="border border-stone-300 p-6"
          onSubmit={handleSubmit}
        >
          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="name">Your Name *</Label>
            <Input
              type="text"
              id="name"
              placeholder="Enter your name"
              required
            />
          </div>

          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="company">Company Name *</Label>
            <Input
              type="text"
              id="company"
              placeholder="Enter your company name"
              required
            />
          </div>

          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="email">Work Email *</Label>
            <Input
              type="email"
              id="email"
              placeholder="you@company.com"
              required
            />
          </div>

          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="problem">What problem are you trying to solve? *</Label>
            <Textarea
              id="problem"
              placeholder="Describe the tasks or workflows you'd like to automate..."
              rows={5}
              required
            />
          </div>

          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="systems">What systems or apps do you use?</Label>
            <Input
              type="text"
              id="systems"
              placeholder="e.g., Slack, Salesforce, Notion, Jira..."
            />
          </div>

          <div className="flex flex-col gap-1.5 mb-4">
            <Label htmlFor="additional">Anything else we should know?</Label>
            <Textarea
              id="additional"
              placeholder="Additional context, timeline, or specific requirements..."
              rows={4}
            />
          </div>

          <button
            type="submit"
            className="w-full px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm cursor-pointer hover:bg-stone-700"
          >
            Submit Request
          </button>
        </form>
      </div>
    </div>
  );
}
