// Lightweight Atlassian Document Format (ADF) → HTML converter.
//
// Used when importing Jira Cloud issues whose `description` field is returned
// as an ADF document (a nested JSON tree) rather than a plain string. We
// convert it to a basic HTML representation so it can be stored and rendered
// with `{@html ...}` like other Jira-imported descriptions.
//
// This is intentionally minimal — it covers the common node and mark types
// found in real-world Jira descriptions. Unknown node types fall back to
// rendering their children, so content is preserved even when the wrapper
// element is unsupported.

type AdfNode = {
  type?: string;
  text?: string;
  attrs?: Record<string, any>;
  marks?: Array<{ type: string; attrs?: Record<string, any> }>;
  content?: AdfNode[];
};

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, c => {
    switch (c) {
      case '&':
        return '&amp;';
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '"':
        return '&quot;';
      case "'":
        return '&#39;';
    }
    return c;
  });
}

function applyMarks(text: string, marks?: AdfNode['marks']): string {
  if (!marks || marks.length === 0) return text;
  let out = text;
  for (const m of marks) {
    switch (m.type) {
      case 'strong':
        out = `<strong>${out}</strong>`;
        break;
      case 'em':
        out = `<em>${out}</em>`;
        break;
      case 'code':
        out = `<code>${out}</code>`;
        break;
      case 'strike':
        out = `<s>${out}</s>`;
        break;
      case 'underline':
        out = `<u>${out}</u>`;
        break;
      case 'subsup': {
        const t = m.attrs?.type === 'sup' ? 'sup' : 'sub';
        out = `<${t}>${out}</${t}>`;
        break;
      }
      case 'link': {
        const href = escapeHtml(String(m.attrs?.href ?? ''));
        out = `<a href="${href}" target="_blank" rel="noopener noreferrer">${out}</a>`;
        break;
      }
      case 'textColor':
      case 'backgroundColor':
      default:
        // Unsupported marks are ignored (text is still rendered).
        break;
    }
  }
  return out;
}

function renderChildren(nodes: AdfNode[] | undefined): string {
  if (!nodes || nodes.length === 0) return '';
  return nodes.map(renderNode).join('');
}

function renderNode(node: AdfNode): string {
  if (!node || typeof node !== 'object') return '';

  switch (node.type) {
    case 'doc':
      return renderChildren(node.content);
    case 'paragraph':
      return `<p>${renderChildren(node.content)}</p>`;
    case 'text':
      return applyMarks(escapeHtml(node.text ?? ''), node.marks);
    case 'hardBreak':
      return '<br/>';
    case 'heading': {
      const level = Math.min(Math.max(Number(node.attrs?.level) || 1, 1), 6);
      return `<h${level}>${renderChildren(node.content)}</h${level}>`;
    }
    case 'bulletList':
      return `<ul>${renderChildren(node.content)}</ul>`;
    case 'orderedList':
      return `<ol>${renderChildren(node.content)}</ol>`;
    case 'listItem':
      return `<li>${renderChildren(node.content)}</li>`;
    case 'codeBlock':
      return `<pre><code>${renderChildren(node.content)}</code></pre>`;
    case 'blockquote':
      return `<blockquote>${renderChildren(node.content)}</blockquote>`;
    case 'rule':
      return '<hr/>';
    case 'panel':
      return `<div class="adf-panel">${renderChildren(node.content)}</div>`;
    case 'table':
      return `<table>${renderChildren(node.content)}</table>`;
    case 'tableRow':
      return `<tr>${renderChildren(node.content)}</tr>`;
    case 'tableCell':
      return `<td>${renderChildren(node.content)}</td>`;
    case 'tableHeader':
      return `<th>${renderChildren(node.content)}</th>`;
    case 'inlineCard':
    case 'blockCard': {
      const url = node.attrs?.url ? String(node.attrs.url) : '';
      if (!url) return '';
      const safe = escapeHtml(url);
      return `<a href="${safe}" target="_blank" rel="noopener noreferrer">${safe}</a>`;
    }
    case 'mention':
      return escapeHtml(String(node.attrs?.text ?? ''));
    case 'emoji':
      return escapeHtml(String(node.attrs?.text ?? node.attrs?.shortName ?? ''));
    case 'mediaSingle':
    case 'mediaGroup':
      return renderChildren(node.content);
    case 'media': {
      const alt = escapeHtml(String(node.attrs?.alt ?? ''));
      const url = node.attrs?.url ? String(node.attrs.url) : '';
      if (url) {
        return `<img src="${escapeHtml(url)}" alt="${alt}"/>`;
      }
      return alt ? `<span>${alt}</span>` : '';
    }
    default:
      // Unknown node — render its children so we don't lose content.
      return renderChildren(node.content);
  }
}

/**
 * Convert an ADF document (or a plain string) to an HTML string.
 *
 * - If `doc` is null/undefined/empty, returns ''.
 * - If `doc` is already a string, returns it unchanged (Jira Data Center
 *   descriptions are returned as plain strings, not ADF).
 * - Otherwise walks the ADF tree and produces basic HTML.
 */
export function adfToHtml(doc: unknown): string {
  if (doc === null || doc === undefined) return '';
  if (typeof doc === 'string') return doc;
  if (typeof doc !== 'object') return '';
  return renderNode(doc as AdfNode);
}
