import { useEffect } from 'react';
import { useGlobalSounds } from '../sounds';

export default function SoundFX() {
  useEffect(() => {
    const cleanup = useGlobalSounds();
    return cleanup;
  }, []);
  return null;
}
