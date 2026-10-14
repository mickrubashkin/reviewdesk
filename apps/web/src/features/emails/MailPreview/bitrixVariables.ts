import type { BitrixVariable } from "../../bitrix-variables/api";

const markerAttribute = "data-rd-variable";

function variableTitle(variable: BitrixVariable) {
  const example = variable.example ? `\nExample: ${variable.example}` : "";
  return `${variable.label}: filled in by Bitrix24 on send${example}`;
}

// markBitrixVariables highlights the board's Bitrix24 variables in the email
// preview and explains them on hover. The text itself is left as is, so
// comment ranges keep their offsets.
export function markBitrixVariables(doc: Document | null | undefined, variables: BitrixVariable[]) {
  if (!doc?.body || variables.length === 0) {
    return;
  }

  for (const element of Array.from(doc.querySelectorAll<HTMLElement>("[href]"))) {
    const href = element.getAttribute("href") ?? "";
    const variable = variables.find((item) => href.includes(item.expression));
    if (variable && !element.hasAttribute(markerAttribute)) {
      element.setAttribute(markerAttribute, "link");
      element.setAttribute("title", variableTitle(variable));
      element.style.outline = "2px dashed #f59f00";
      element.style.outlineOffset = "2px";
    }
  }

  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT);
  const textNodes: Text[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const parent = node.parentElement;
    if (parent && !parent.hasAttribute(markerAttribute) && variables.some((item) => node.nodeValue?.includes(item.expression))) {
      textNodes.push(node as Text);
    }
  }
  for (const textNode of textNodes) {
    wrapVariablesInTextNode(doc, textNode, variables);
  }
}

function wrapVariablesInTextNode(doc: Document, textNode: Text, variables: BitrixVariable[]) {
  let current: Text | null = textNode;
  while (current) {
    const value: string = current.nodeValue ?? "";
    let match: { index: number; variable: BitrixVariable } | null = null;
    for (const variable of variables) {
      const index = value.indexOf(variable.expression);
      if (index >= 0 && (!match || index < match.index)) {
        match = { index, variable };
      }
    }
    if (!match) {
      return;
    }
    const variableNode = current.splitText(match.index);
    const rest = variableNode.splitText(match.variable.expression.length);
    const mark = doc.createElement("span");
    mark.setAttribute(markerAttribute, "text");
    mark.setAttribute("title", variableTitle(match.variable));
    mark.style.background = "#fff3bf";
    mark.style.borderRadius = "3px";
    variableNode.replaceWith(mark);
    mark.appendChild(variableNode);
    current = rest.nodeValue ? rest : null;
  }
}
