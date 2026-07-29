import Markdown from "react-markdown";
import type { PluggableList } from "unified";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import remarkUnwrapImages from "@/lib/remark-unwrap-images";
import { cn } from "@/lib/utils";

/**
 * Extends rehype-sanitize's default schema (already GitHub-modeled) with the
 * additional tags and attributes that GitHub's html-pipeline allows.
 * See: https://github.com/gjtorikian/html-pipeline
 */
const githubSchema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []),
    "abbr",
    "bdo",
    "caption",
    "cite",
    "dfn",
    "figure",
    "figcaption",
    "mark",
    "small",
    "time",
    "wbr",
  ],
  attributes: {
    ...defaultSchema.attributes,
    img: [...(defaultSchema.attributes?.img ?? []), "loading"],
    time: [...(defaultSchema.attributes?.time ?? []), "dateTime"],
  },
  strip: ["script", "style"],
};

export interface StyledMarkdownProps {
  children: string;
  className?: string;
}

export const proseClasses = [
  "prose prose-stone dark:prose-invert prose-sm max-w-none overflow-x-auto",
  "font-sans text-body text-foreground",
  // headings
  "prose-headings:font-semibold prose-headings:text-foreground",
  "prose-h1:text-[22px] prose-h1:mt-8 prose-h1:mb-3 prose-h1:pb-2 prose-h1:border-b prose-h1:border-border-strong",
  "prose-h2:text-[17px] prose-h2:mt-7 prose-h2:mb-2.5 prose-h2:pb-2 prose-h2:border-b prose-h2:border-border-strong",
  "prose-h3:text-[15px] prose-h3:font-semibold prose-h3:mt-5 prose-h3:mb-1.5",
  "prose-h4:text-[14px] prose-h4:font-semibold prose-h4:mt-4 prose-h4:mb-1",
  "prose-h5:text-[13px] prose-h5:font-semibold prose-h5:mt-4 prose-h5:mb-1",
  "prose-h6:text-[13px] prose-h6:font-semibold prose-h6:mt-4 prose-h6:mb-1 prose-h6:text-[var(--muted-foreground)]",
  // body
  "prose-p:my-2.5",
  "prose-ul:my-1.5 prose-ul:pl-4 prose-ol:my-1.5 prose-ol:pl-6 [&_ol]:list-decimal [&_ul]:marker:text-foreground [&_ol]:marker:text-foreground",
  "prose-li:my-0.5 prose-li:text-body",
  "prose-a:text-foreground prose-a:underline prose-a:decoration-muted-foreground/50 prose-a:underline-offset-4 hover:prose-a:decoration-foreground",
  "prose-strong:font-semibold prose-strong:text-foreground",
  "prose-blockquote:my-3 prose-blockquote:border-border-strong prose-hr:my-4",
  // task list checkboxes
  "[&_.contains-task-list]:list-none [&_.contains-task-list]:pl-0",
  "[&_input[type=checkbox]]:accent-primary [&_input[type=checkbox]]:mr-2 [&_input[type=checkbox]]:align-middle [&_input[type=checkbox]]:relative [&_input[type=checkbox]]:-top-px",
  // tables
  "[&_table]:border-collapse [&_th]:border [&_th]:border-border [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-[12px] [&_th]:font-semibold [&_th]:text-foreground [&_th]:bg-muted dark:[&_th]:bg-foreground/5 [&_td]:border [&_td]:border-border [&_td]:px-3 [&_td]:py-1.5 [&_td]:text-[12px]",
  // code blocks
  "prose-pre:my-3.5 prose-pre:rounded-[4px] prose-pre:bg-muted dark:prose-pre:bg-foreground/5 prose-pre:text-foreground prose-pre:leading-[1.8] [&_pre_code]:text-[12.5px]",
  // inline code
  "[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-muted dark:[&_:not(pre)>code]:bg-foreground/5 [&_:not(pre)>code]:border [&_:not(pre)>code]:border-border",
  "[&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-xs [&_:not(pre)>code]:text-foreground",
  "[&_:not(pre)>code]:font-normal [&_:not(pre)>code]:before:content-[''] [&_:not(pre)>code]:after:content-['']",
  // images: block images get max-width; inline images align with text (playground#28)
  "prose-img:max-w-full prose-img:rounded prose-img:align-middle",
  "[&>img]:inline [&>a>img]:inline",
  "[&_p_img]:m-0",
].join(" ");

const remarkPlugins = [remarkGfm, remarkUnwrapImages];
const rehypePlugins: PluggableList = [rehypeRaw, [rehypeSanitize, githubSchema]];

const markdownComponents = {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  input: ({ node, ...props }: React.ComponentPropsWithoutRef<"input"> & { node?: unknown }) => (
    <input {...props} readOnly />
  ),
};

export function StyledMarkdown({ children, className }: StyledMarkdownProps) {
  return (
    <div className={cn(proseClasses, className)}>
      <Markdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={markdownComponents}>
        {children}
      </Markdown>
    </div>
  );
}
