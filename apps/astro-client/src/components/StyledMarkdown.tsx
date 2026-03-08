import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";

export interface StyledMarkdownProps {
  children: string;
  className?: string;
}

const proseClasses = [
  "prose prose-stone dark:prose-invert prose-sm max-w-none overflow-x-auto",
  "text-[13px] leading-[1.75] text-[var(--ink-muted)]",
  // headings
  "prose-headings:font-bold prose-headings:text-primary",
  "prose-h1:text-[22px] prose-h1:mt-8 prose-h1:mb-3 prose-h1:pb-2 prose-h1:border-b prose-h1:border-border-strong",
  "prose-h2:text-[17px] prose-h2:mt-7 prose-h2:mb-2.5 prose-h2:pb-2 prose-h2:border-b prose-h2:border-border-strong",
  "prose-h3:text-[15px] prose-h3:font-semibold prose-h3:mt-5 prose-h3:mb-1.5",
  "prose-h4:text-[14px] prose-h4:font-semibold prose-h4:mt-4 prose-h4:mb-1",
  "prose-h5:text-[13px] prose-h5:font-semibold prose-h5:mt-4 prose-h5:mb-1",
  "prose-h6:text-[13px] prose-h6:font-semibold prose-h6:mt-4 prose-h6:mb-1 prose-h6:text-[var(--ink-muted)]",
  // body
  "prose-p:my-2.5",
  "prose-ul:my-1.5 prose-ul:pl-4 prose-ol:my-1.5 [&_ul]:marker:text-teal-500 [&_ol]:marker:text-teal-700",
  "prose-li:my-0.5 prose-li:text-[13px] prose-li:leading-[1.7]",
  "prose-strong:font-semibold prose-strong:text-foreground",
  "prose-blockquote:my-3 prose-blockquote:border-teal-400 prose-hr:my-4",
  // task list checkboxes
  "[&_.contains-task-list]:list-none [&_.contains-task-list]:pl-0",
  "[&_input[type=checkbox]]:accent-teal-600 [&_input[type=checkbox]]:mr-2 [&_input[type=checkbox]]:align-middle [&_input[type=checkbox]]:relative [&_input[type=checkbox]]:-top-px",
  // tables
  "[&_table]:border-collapse [&_th]:border [&_th]:border-stone-300 [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-[12px] [&_th]:font-semibold [&_th]:text-foreground [&_th]:bg-stone-200 [&_td]:border [&_td]:border-stone-300 [&_td]:px-3 [&_td]:py-1.5 [&_td]:text-[12px] dark:[&_th]:border-border dark:[&_td]:border-border dark:[&_th]:bg-teal-900/30",
  // code blocks
  "prose-pre:my-3.5 prose-pre:rounded-md prose-pre:bg-teal-900 prose-pre:text-code-text prose-pre:leading-[1.8] [&_pre_code]:text-[12.5px]",
  // inline code
  "[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-stone-300 [&_:not(pre)>code]:border [&_:not(pre)>code]:border-border-strong",
  "[&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-xs [&_:not(pre)>code]:text-primary",
  "[&_:not(pre)>code]:font-normal [&_:not(pre)>code]:before:content-[''] [&_:not(pre)>code]:after:content-['']",
  "dark:[&_:not(pre)>code]:bg-teal-900/40 dark:[&_:not(pre)>code]:border-teal-300/20 dark:[&_:not(pre)>code]:text-teal-300",
].join(" ");

const remarkPlugins = [remarkGfm];

const markdownComponents = {
  input: ({ node: _, ...props }: React.ComponentPropsWithoutRef<"input"> & { node?: unknown }) => (
    <input {...props} readOnly />
  ),
};

export function StyledMarkdown({ children, className }: StyledMarkdownProps) {
  return (
    <div className={cn(proseClasses, className)}>
      <Markdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {children}
      </Markdown>
    </div>
  );
}
