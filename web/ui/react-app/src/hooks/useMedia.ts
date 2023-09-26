import { useEffect, useState } from 'react';

// A hook to determine whether a CSS media query finds any matches.
const useMedia = (query: string): boolean => {
  const mediaQuery = window.matchMedia(query);
  const [matches, setMatches] = useState(mediaQuery.matches);

  useEffect(() => {
    const handler = () => setMatches(mediaQuery.matches);
    mediaQuery.addEventListener('change', handler);
    return () => mediaQuery.removeEventListener('change', handler);
  }, [mediaQuery]);

  return matches;
};

export default useMedia;
