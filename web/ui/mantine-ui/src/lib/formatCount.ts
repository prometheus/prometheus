// Formats a count for display, grouping thousands according to the browser
// locale (e.g. 1234567 -> "1,234,567").
export const formatCount = (n: number): string => n.toLocaleString();
