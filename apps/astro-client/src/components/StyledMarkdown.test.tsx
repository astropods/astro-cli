import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { StyledMarkdown } from "./StyledMarkdown";

/** Render markdown and return the inner HTML of the wrapper div. */
function renderMarkdown(md: string) {
  const { container } = render(<StyledMarkdown>{md}</StyledMarkdown>);
  return container.innerHTML;
}

describe("StyledMarkdown sanitization", () => {
  // ── Script injection ────────────────────────────────────────────────

  it("strips <script> tags", () => {
    const html = renderMarkdown('<script>alert("xss")</script>');
    expect(html).not.toContain("<script");
    expect(html).not.toContain("alert");
  });

  it("strips <script> with src attribute", () => {
    const html = renderMarkdown('<script src="https://evil.com/xss.js"></script>');
    expect(html).not.toContain("<script");
    expect(html).not.toContain("evil.com");
  });

  it("strips <script> disguised with mixed case", () => {
    const html = renderMarkdown('<ScRiPt>alert("xss")</ScRiPt>');
    expect(html).not.toContain("<script");
    expect(html).not.toContain("<ScRiPt");
    expect(html).not.toContain("alert");
  });

  // ── Event handler injection ─────────────────────────────────────────

  it("strips onclick handlers", () => {
    const html = renderMarkdown('<div onclick="alert(1)">click me</div>');
    expect(html).not.toContain("onclick");
    expect(html).not.toContain("alert");
    expect(html).toContain("click me");
  });

  it("strips onerror on img tags", () => {
    const html = renderMarkdown('<img src="x" onerror="alert(1)">');
    expect(html).not.toContain("onerror");
    expect(html).not.toContain("alert");
  });

  it("strips onload handlers", () => {
    const html = renderMarkdown('<body onload="alert(1)">');
    expect(html).not.toContain("onload");
    expect(html).not.toContain("alert");
  });

  it("strips onmouseover handlers", () => {
    const html = renderMarkdown('<a onmouseover="alert(1)">hover</a>');
    expect(html).not.toContain("onmouseover");
    expect(html).not.toContain("alert");
  });

  it("strips onfocus with autofocus trick", () => {
    const html = renderMarkdown('<input onfocus="alert(1)" autofocus>');
    expect(html).not.toContain("onfocus");
    expect(html).not.toContain("alert");
  });

  // ── javascript: URI injection ───────────────────────────────────────

  it("strips javascript: hrefs", () => {
    const html = renderMarkdown('<a href="javascript:alert(1)">click</a>');
    expect(html).not.toContain("javascript:");
  });

  it("strips javascript: hrefs with entity encoding", () => {
    const html = renderMarkdown('<a href="&#106;avascript:alert(1)">click</a>');
    expect(html).not.toContain("alert");
  });

  it("strips javascript: hrefs with whitespace padding", () => {
    const html = renderMarkdown('<a href=" javascript:alert(1)">click</a>');
    expect(html).not.toContain("javascript");
  });

  it("strips javascript: in img src", () => {
    const html = renderMarkdown('<img src="javascript:alert(1)">');
    expect(html).not.toContain("javascript:");
  });

  // ── Dangerous tags ──────────────────────────────────────────────────

  it("strips <iframe> tags", () => {
    const html = renderMarkdown('<iframe src="https://evil.com"></iframe>');
    expect(html).not.toContain("<iframe");
    expect(html).not.toContain("evil.com");
  });

  it("strips <object> tags", () => {
    const html = renderMarkdown('<object data="https://evil.com/flash.swf"></object>');
    expect(html).not.toContain("<object");
  });

  it("strips <embed> tags", () => {
    const html = renderMarkdown('<embed src="https://evil.com/flash.swf">');
    expect(html).not.toContain("<embed");
  });

  it("strips <form> tags", () => {
    const html = renderMarkdown('<form action="https://evil.com"><input type="text"></form>');
    expect(html).not.toContain("<form");
  });

  it("strips <style> tags", () => {
    const html = renderMarkdown("<style>body { display: none }</style>");
    expect(html).not.toContain("<style");
    expect(html).not.toContain("display: none");
  });

  it("strips <link> tags", () => {
    const html = renderMarkdown('<link rel="stylesheet" href="https://evil.com/style.css">');
    expect(html).not.toContain("<link");
  });

  it("strips <meta> tags", () => {
    const html = renderMarkdown('<meta http-equiv="refresh" content="0;url=https://evil.com">');
    expect(html).not.toContain("<meta");
  });

  it("strips <base> tags", () => {
    const html = renderMarkdown('<base href="https://evil.com/">');
    expect(html).not.toContain("<base");
  });

  // ── data: URI injection ─────────────────────────────────────────────

  it("strips data: URIs on img src", () => {
    const html = renderMarkdown(
      '<img src="data:text/html,<script>alert(1)</script>">',
    );
    expect(html).not.toContain("data:text/html");
    expect(html).not.toContain("alert");
  });

  it("strips data: URIs on anchor href", () => {
    const html = renderMarkdown(
      '<a href="data:text/html,<script>alert(1)</script>">click</a>',
    );
    expect(html).not.toContain("data:");
  });

  // ── SVG / MathML injection ──────────────────────────────────────────

  it("strips <svg> with embedded script", () => {
    const html = renderMarkdown(
      '<svg onload="alert(1)"><circle r="40"></circle></svg>',
    );
    expect(html).not.toContain("<svg");
    expect(html).not.toContain("onload");
    expect(html).not.toContain("alert");
  });

  it("strips <math> with xlink payload", () => {
    const html = renderMarkdown(
      '<math><maction actiontype="statusline#http://evil.com">click</maction></math>',
    );
    expect(html).not.toContain("<math");
    expect(html).not.toContain("evil.com");
  });

  // ── Attribute injection via allowed tags ─────────────────────────────

  it("strips style attributes", () => {
    const html = renderMarkdown(
      '<div style="background:url(javascript:alert(1))">text</div>',
    );
    expect(html).not.toContain("style=");
    expect(html).not.toContain("javascript:");
  });

  it("strips class attribute used for CSS injection", () => {
    const html = renderMarkdown('<p class="malicious-class">text</p>');
    expect(html).not.toContain('class="malicious-class"');
    expect(html).toContain("text");
  });

  // ── Nested / combined attacks ───────────────────────────────────────

  it("strips script inside allowed tags", () => {
    const html = renderMarkdown(
      '<details><summary>Click</summary><script>alert(1)</script></details>',
    );
    expect(html).toContain("<details");
    expect(html).toContain("<summary");
    expect(html).not.toContain("<script");
    expect(html).not.toContain("alert");
  });

  it("strips nested event handlers in deep HTML", () => {
    const html = renderMarkdown(
      '<div><p><a href="https://ok.com" onclick="alert(1)">link</a></p></div>',
    );
    expect(html).not.toContain("onclick");
    expect(html).toContain("link");
  });

  it("strips img onerror inside markdown emphasis", () => {
    const html = renderMarkdown(
      'Text *bold* <img src=x onerror=alert(1)> more text',
    );
    expect(html).not.toContain("onerror");
    expect(html).not.toContain("alert");
  });

  // ── Null byte injection ──────────────────────────────────────────────

  it("strips script tag with null byte in tag name", () => {
    // The null byte makes the parser see <scr�ipt>, not <script>.
    // The text leaks as plain text — no script element is created.
    const html = renderMarkdown('<scr\x00ipt>alert(1)</scr\x00ipt>');
    expect(html).not.toContain("<script>");
    expect(html).not.toContain("</script>");
  });

  it("strips null byte in attribute name", () => {
    // hr\0ef becomes hr�ef — not a real href, so no functional link.
    const { container } = render(
      <StyledMarkdown>{'<a hr\x00ef="javascript:alert(1)">click</a>'}</StyledMarkdown>,
    );
    const links = container.querySelectorAll("a[href]");
    links.forEach((link) => {
      expect(link.getAttribute("href")).not.toMatch(/^javascript:/i);
    });
  });

  // ── Tab / newline / CR in protocols ─────────────────────────────────

  it("strips javascript: with embedded tab", () => {
    const html = renderMarkdown('<a href="java\tscript:alert(1)">click</a>');
    expect(html).not.toContain("alert");
  });

  it("strips javascript: with embedded newline", () => {
    const html = renderMarkdown('<a href="java\nscript:alert(1)">click</a>');
    expect(html).not.toContain("alert");
  });

  it("strips javascript: with embedded carriage return", () => {
    const html = renderMarkdown('<a href="java\rscript:alert(1)">click</a>');
    expect(html).not.toContain("alert");
  });

  it("strips javascript: with mixed whitespace characters", () => {
    const html = renderMarkdown('<a href="j\ta\nv\ra\tscript:alert(1)">click</a>');
    expect(html).not.toContain("alert");
  });

  // ── HTML entity encoding tricks ─────────────────────────────────────

  it("strips javascript: built with hex entities", () => {
    const html = renderMarkdown(
      '<a href="&#x6A;&#x61;&#x76;&#x61;&#x73;&#x63;&#x72;&#x69;&#x70;&#x74;&#x3A;alert(1)">click</a>',
    );
    expect(html).not.toContain("alert");
  });

  it("strips javascript: built with decimal entities", () => {
    const html = renderMarkdown(
      '<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">click</a>',
    );
    expect(html).not.toContain("alert");
  });

  it("strips javascript: with zero-padded entities", () => {
    const html = renderMarkdown(
      '<a href="&#0000106;avascript:alert(1)">click</a>',
    );
    expect(html).not.toContain("alert");
  });

  it("strips javascript: with mixed entity and plain chars", () => {
    const html = renderMarkdown(
      '<a href="j&#97;vascr&#x69;pt:alert(1)">click</a>',
    );
    expect(html).not.toContain("alert");
  });

  // ── Markdown-native protocol injection ──────────────────────────────

  it("strips javascript: in markdown link syntax", () => {
    const html = renderMarkdown("[click me](javascript:alert(1))");
    expect(html).not.toContain("javascript:");
  });

  it("strips javascript: in markdown image syntax", () => {
    const html = renderMarkdown("![alt](javascript:alert(1))");
    expect(html).not.toContain("javascript:");
  });

  it("strips vbscript: in markdown link syntax", () => {
    const html = renderMarkdown("[click](vbscript:alert(1))");
    expect(html).not.toContain("vbscript:");
  });

  it("strips data: URI in markdown link syntax", () => {
    const html = renderMarkdown(
      "[click](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
    );
    expect(html).not.toContain("data:");
  });

  it("strips data: URI in markdown image syntax", () => {
    const html = renderMarkdown(
      "![img](data:text/html,<script>alert(1)</script>)",
    );
    expect(html).not.toContain("data:text/html");
  });

  // ── Mutation XSS (mXSS) patterns ───────────────────────────────────

  it("strips noscript-based mXSS vector", () => {
    const html = renderMarkdown(
      '<noscript><img src=x onerror="alert(1)"></noscript>',
    );
    expect(html).not.toContain("onerror");
    expect(html).not.toContain("alert");
  });

  it("strips textarea-based mXSS vector", () => {
    // <textarea> treats inner content as text, so the img becomes escaped
    // text like &lt;img...&gt; — no real img element is created.
    const { container } = render(
      <StyledMarkdown>{'<textarea><img src=x onerror="alert(1)"></textarea>'}</StyledMarkdown>,
    );
    const images = container.querySelectorAll("img[onerror]");
    expect(images).toHaveLength(0);
  });

  it("strips title-based mXSS vector", () => {
    // <title> treats its content as text, so the img tag is HTML-escaped
    // and appears as visible text — not a real DOM element.
    const { container } = render(
      <StyledMarkdown>{'<title><img src=x onerror="alert(1)"></title>'}</StyledMarkdown>,
    );
    const images = container.querySelectorAll("img[onerror]");
    expect(images).toHaveLength(0);
  });

  // ── HTML comment / CDATA tricks ─────────────────────────────────────

  it("strips script hidden after comment close", () => {
    const html = renderMarkdown('<!--><script>alert(1)</script>');
    expect(html).not.toContain("<script");
    expect(html).not.toContain("alert");
  });

  it("strips CDATA wrapper around script", () => {
    // HTML5 treats <![CDATA[ as a comment, so <script> inside is consumed.
    // Any remaining text is plain text, not a script element.
    const { container } = render(
      <StyledMarkdown>{'<![CDATA[<script>alert(1)</script>]]>'}</StyledMarkdown>,
    );
    expect(container.querySelectorAll("script")).toHaveLength(0);
  });

  // ── Unquoted / malformed attributes ─────────────────────────────────

  it("strips unquoted event handler attributes", () => {
    const html = renderMarkdown("<img src=x onerror=alert(1)>");
    expect(html).not.toContain("onerror");
    expect(html).not.toContain("alert");
  });

  it("strips backtick-quoted attributes", () => {
    // Backticks aren't valid attribute delimiters in HTML5; the parser
    // includes them in the value, making it `javascript:... not javascript:...
    // Verify no img element has a functional javascript: src.
    const { container } = render(
      <StyledMarkdown>{"<img src=`javascript:alert(1)`>"}</StyledMarkdown>,
    );
    const images = container.querySelectorAll("img");
    images.forEach((img) => {
      const src = img.getAttribute("src") ?? "";
      expect(src).not.toMatch(/^javascript:/i);
    });
  });

  it("strips event handler after attribute value breakout", () => {
    const html = renderMarkdown(
      '<a href="x" onclick="alert(1)" href="https://safe.com">link</a>',
    );
    expect(html).not.toContain("onclick");
    expect(html).not.toContain("alert");
  });

  // ── SVG / foreignObject / use tricks ────────────────────────────────

  it("strips SVG foreignObject with embedded HTML", () => {
    const html = renderMarkdown(
      '<svg><foreignObject><body onload="alert(1)"></body></foreignObject></svg>',
    );
    expect(html).not.toContain("<svg");
    expect(html).not.toContain("onload");
    expect(html).not.toContain("alert");
  });

  it("strips SVG use with data URI xlink:href", () => {
    const html = renderMarkdown(
      '<svg><use xlink:href="data:image/svg+xml,<svg onload=alert(1)>"></use></svg>',
    );
    expect(html).not.toContain("<svg");
    expect(html).not.toContain("alert");
  });

  it("strips SVG animate with event handler", () => {
    const html = renderMarkdown(
      '<svg><animate onbegin="alert(1)" attributeName="x"></animate></svg>',
    );
    expect(html).not.toContain("onbegin");
    expect(html).not.toContain("alert");
  });

  // ── Other protocol schemes ──────────────────────────────────────────

  it("strips vbscript: in href", () => {
    const html = renderMarkdown('<a href="vbscript:MsgBox(1)">click</a>');
    expect(html).not.toContain("vbscript:");
  });

  it("strips file: protocol in href", () => {
    const html = renderMarkdown('<a href="file:///etc/passwd">click</a>');
    expect(html).not.toContain("file:");
  });

  // ── Unicode / encoding tricks ───────────────────────────────────────

  it("strips fullwidth unicode javascript: in href", () => {
    // \uFF4A is fullwidth 'ｊ' — browsers don't treat ｊavascript: as javascript:
    // Verify no anchor has a functional javascript: href.
    const { container } = render(
      <StyledMarkdown>{'<a href="\uFF4Aavascript:alert(1)">click</a>'}</StyledMarkdown>,
    );
    const links = container.querySelectorAll("a[href]");
    links.forEach((link) => {
      expect(link.getAttribute("href")).not.toMatch(/^javascript:/i);
    });
  });

  it("strips UTF-7 encoded script attempt", () => {
    const html = renderMarkdown("+ADw-script+AD4-alert(1)+ADw-/script+AD4-");
    expect(html).not.toContain("<script");
  });

  it("strips percent-encoded javascript: in href", () => {
    const html = renderMarkdown(
      '<a href="%6A%61%76%61%73%63%72%69%70%74:alert(1)">click</a>',
    );
    expect(html).not.toContain("alert(1)");
  });

  // ── Allowed tags still render ───────────────────────────────────────

  it("renders <details> and <summary>", () => {
    const html = renderMarkdown(
      "<details><summary>Expand</summary>\n\nHidden content\n\n</details>",
    );
    expect(html).toContain("<details");
    expect(html).toContain("<summary");
    expect(html).toContain("Hidden content");
  });

  it("renders <kbd>, <sub>, <sup>", () => {
    const html = renderMarkdown(
      "Press <kbd>Ctrl</kbd>+<kbd>C</kbd>. H<sub>2</sub>O is x<sup>2</sup>.",
    );
    expect(html).toContain("<kbd>");
    expect(html).toContain("<sub>");
    expect(html).toContain("<sup>");
  });

  it("renders <mark> for highlights", () => {
    const html = renderMarkdown("This is <mark>important</mark>.");
    expect(html).toContain("<mark>");
    expect(html).toContain("important");
  });

  it("renders <abbr> with title", () => {
    const html = renderMarkdown(
      '<abbr title="HyperText Markup Language">HTML</abbr>',
    );
    expect(html).toContain("<abbr");
    expect(html).toContain("HTML");
  });

  it("renders <figure> and <figcaption>", () => {
    const html = renderMarkdown(
      '<figure><img src="https://example.com/img.png" alt="photo"><figcaption>Caption</figcaption></figure>',
    );
    expect(html).toContain("<figure");
    expect(html).toContain("<figcaption");
    expect(html).toContain("Caption");
  });

  it("renders <dl>, <dt>, <dd> for definition lists", () => {
    const html = renderMarkdown(
      "<dl><dt>Term</dt><dd>Definition</dd></dl>",
    );
    expect(html).toContain("<dl>");
    expect(html).toContain("<dt>");
    expect(html).toContain("<dd>");
  });
});
