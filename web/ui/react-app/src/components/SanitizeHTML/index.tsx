/**
 * SanitizeHTML to render HTML, this takes care of sanitizing HTML.
 */
import React, { memo } from 'react';
import sanitizeHTML from 'sanitize-html';

interface SanitizeHTMLProps {
  allowedTags: string[];
  children: string;
  tag?: keyof JSX.IntrinsicElements;
}

const SanitizeHTML = ({ tag: Tag = 'div', children, allowedTags, ...rest }: SanitizeHTMLProps) => (
  <Tag {...rest} dangerouslySetInnerHTML={{ __html: sanitizeHTML(children, { allowedTags }) }} />
);

export default memo(SanitizeHTML);
