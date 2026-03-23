import { useState } from "react";
import { CheckCircleIcon } from "@heroicons/react/24/outline";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

interface FeedbackModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FeedbackModal({ open, onOpenChange }: FeedbackModalProps) {
  const [message, setMessage] = useState("");
  const [submitted, setSubmitted] = useState(false);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    // TODO: wire up submission endpoint
    setSubmitted(true);
  }

  function handleOpenChange(next: boolean) {
    onOpenChange(next);
    if (!next) {
      setMessage("");
      setSubmitted(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        {submitted ? (
          <div className="flex flex-col items-center gap-4 py-4 text-center">
            <div className="rounded-xl bg-teal-100 p-3 dark:bg-teal-900/50">
              <CheckCircleIcon className="size-7 text-teal-600 dark:text-teal-400" />
            </div>
            <div className="flex flex-col gap-1.5">
              <p className="text-base font-semibold text-foreground">Thank you for sharing!</p>
              <p className="text-sm text-muted-foreground">
                We read every submission and truly value your feedback.
              </p>
            </div>
          </div>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Share feedback</DialogTitle>
              <DialogDescription>
                What's working well, what's not, or what you'd love to see next.
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
              <Textarea
                placeholder="Your feedback…"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                rows={5}
                className="resize-none"
                autoFocus
              />
              <div className="flex justify-end gap-2">
                <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={!message.trim()}>
                  Send feedback
                </Button>
              </div>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
