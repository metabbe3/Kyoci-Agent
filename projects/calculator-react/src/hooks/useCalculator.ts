// This is a custom hook for calculator logic
import { useState } from 'react';

export const useCalculator = () => {
  const [result, setResult] = useState('0');

  const handleDigitClick = (digit: string) => {
    if (result === '0') {
      setResult(digit);
    } else {
      setResult(result + digit);
    }
  };

  const handleClearClick = () => {
    setResult('0');
  };

  const handleEqualsClick = () => {
    try {
      setResult(eval(result).toString());
    } catch (error) {
      setResult('Error');
    }
  };

  return {
    result,
    handleDigitClick,
    handleClearClick,
    handleEqualsClick,
  };
};
