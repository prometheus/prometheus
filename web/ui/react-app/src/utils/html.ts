export const escapeHTML = (str: string): string => {
  const entityMap: { [key: string]: string } = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
    '/': '&#x2F;',
  };

  return String(str).replace(/[&<>"'/]/g, function(s) {
    return entityMap[s];
  });
};
