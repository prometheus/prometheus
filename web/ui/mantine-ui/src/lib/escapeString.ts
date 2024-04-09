// Used for escaping escape sequences and double quotes in double-quoted strings.
export const escapeString = (str: string) => {
  return str.replace(/([\\"])/g, "\\$1");
};
