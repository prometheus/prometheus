export const escapeString = (str: string) => {
  return str.replace(/([\\"])/g, "\\$1");
};
