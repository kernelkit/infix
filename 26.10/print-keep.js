/* Keep a heading on the same page as what follows it when printing.
 *
 * Browsers cannot keep siblings together, and Firefox ignores
 * break-after: avoid, so before printing each heading is wrapped with
 * its first block, plus a short introduction and the figure, table or
 * code block it introduces, in a div that must not break.  The wrap is
 * undone after printing. */
(function () {
  const BLOCK = "figure, table, .md-typeset__scrollwrap, .highlight, pre, .admonition, details";
  let groups = [];

  /* A short paragraph, or one ending in a colon, introduces what follows */
  function intro(el) {
    if (!el || el.tagName !== "P")
      return false;
    const text = el.textContent.trim();
    return text.endsWith(":") || text.length < 240;
  }

  function keep() {
    unkeep();
    const heads = document.querySelectorAll(".md-typeset :is(h1, h2, h3, h4, h5, h6)");
    for (const h of Array.from(heads).reverse()) {
      const g = document.createElement("div");
      let next = h.nextElementSibling;

      g.className = "print-keep";
      h.replaceWith(g);
      g.appendChild(h);
      groups.push(g);
      if (!next)
        continue;

      g.appendChild(next);
      while (intro(next)) {
        next = g.nextElementSibling;
        if (!next || !(intro(next) || next.matches(BLOCK)))
          break;
        g.appendChild(next);
      }
    }
  }

  function unkeep() {
    for (const g of groups)
      g.replaceWith(...g.childNodes);
    groups = [];
  }

  addEventListener("beforeprint", keep);
  addEventListener("afterprint", unkeep);
})();
