export const generateID = () => {
  return `_${Math.random()
    .toString(36)
    .substr(2, 9)}`;
};

export const byEmptyString = (p: string) => p.length > 0;

export const isPresent = <T>(obj: T): obj is NonNullable<T> => obj !== null && obj !== undefined;
