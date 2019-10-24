export const uuidGen = () =>
  '_' +
  Math.random()
    .toString(36)
    .substr(2, 9);
