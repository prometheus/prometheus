/**
 * SanitizeHTML to render HTML, this takes care of sanitizing HTML.
 */
import React, { PureComponent } from 'react';
import sanitizeHTML from 'sanitize-html';

interface SanitizeHTMLProps {
  inline: Boolean;
  allowedTags: string[];
  children: Element | string;
}

class SanitizeHTML extends PureComponent<SanitizeHTMLProps> {
  sanitize = (html: any) => {
    return sanitizeHTML(html, {
      allowedTags: this.props.allowedTags
    });
  };

  render() {
    const { inline, children } = this.props;
    return inline ? (
      <span dangerouslySetInnerHTML={{ __html: this.sanitize(children) }} />
    ) : (
      <div dangerouslySetInnerHTML={{ __html: this.sanitize(children) }} />
    );
  }
}

export default SanitizeHTML;
