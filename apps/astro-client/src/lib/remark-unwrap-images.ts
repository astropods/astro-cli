/**
 * Remark plugin that unwraps images from paragraphs when a paragraph contains
 * only images (and optional link wrappers around images). This matches GitHub's
 * rendering behavior where consecutive images without blank lines render inline.
 *
 * Before: <p><img /><img /></p>  (block, stacked)
 * After:  <img /><img />         (inline, side-by-side)
 */

import type { Root, Paragraph, PhrasingContent } from "mdast";
import type { Plugin } from "unified";
import { visit } from "unist-util-visit";

function isImageNode(node: PhrasingContent): boolean {
  if (node.type === "image") return true;
  // A link wrapping only an image counts as an image (e.g. badge links)
  if (
    node.type === "link" &&
    node.children.length === 1 &&
    node.children[0].type === "image"
  ) {
    return true;
  }
  return false;
}

function isImageOnlyParagraph(node: Paragraph): boolean {
  // Every child must be an image or a link-wrapped image
  // (ignoring whitespace-only text nodes between them)
  return node.children.every(
    (child) =>
      isImageNode(child) ||
      (child.type === "text" && child.value.trim() === ""),
  );
}

const remarkUnwrapImages: Plugin<[], Root> = () => {
  return (tree) => {
    visit(tree, "paragraph", (node: Paragraph, index, parent) => {
      if (index == null || !parent) return;
      if (!isImageOnlyParagraph(node)) return;

      // Replace the paragraph with its children directly
      parent.children.splice(index, 1, ...node.children);

      // Return the current index so the visitor doesn't skip the spliced nodes
      return index;
    });
  };
};

export default remarkUnwrapImages;
