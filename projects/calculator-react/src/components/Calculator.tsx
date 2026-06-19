// Calculator.tsx
import React, { useState } from 'react';
import './Calculator.css';

interface CalculatorProps {
  onResult: (result: string) => void;
}

const Calculator: React.FC<CalculatorProps> = ({ onResult }) => {
  const [display, setDisplay] = useState('0');

  const handleNumberClick = (num: string) => {
    setDisplay((prev) => prev === '0' ? num : prev + num);
  };

  const handleClearClick = () => {
    setDisplay('0');
  };

  const handleEqualsClick = () => {
    try {
      onResult(eval(display));
    } catch (error) {
      setDisplay('Error');
    }
  };

  return (
    <div className="calculator">
      <input type="text" value={display} readOnly />
      <div className="buttons">
        <button onClick={() => handleNumberClick('7')}>7</button>
        <button onClick={() => handleNumberClick('8')}>8</button>
        <button onClick={() => handleNumberClick('9')}>9</button>
        <button onClick={() => handleClearClick()}>C</button>
        <button onClick={() => handleNumberClick('4')}>4</button>
        <button onClick={() => handleNumberClick('5')}>5</button>
        <button onClick={() => handleNumberClick('6')}>6</button>
        <button onClick={() => handleNumberClick('*')}>*</button>
        <button onClick={() => handleNumberClick('1')}>1</button>
        <button onClick={() => handleNumberClick('2')}>2</button>
        <button onClick={() => handleNumberClick('3')}>3</button>
        <button onClick={() => handleNumberClick('-')}>-</button>
        <button onClick={() => handleNumberClick('0')}>0</button>
        <button onClick={() => handleNumberClick('.')}>.</button>
        <button onClick={() => handleNumberClick('+')}>+</button>
        <button onClick={handleEqualsClick}>=</button>
      </div>
    </div>
  );
};

export default Calculator;
