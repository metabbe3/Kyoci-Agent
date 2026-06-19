import React, { useState } from 'react';

interface CalculatorProps {
  initialResult: number;
}

const Calculator: React.FC<CalculatorProps> = ({ initialResult }) => {
  const [result, setResult] = useState(initialResult);

  const handleNumberClick = (number: number) => {
    setResult((prevResult) => prevResult * 10 + number);
  };

  const handleClearClick = () => {
    setResult(0);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      <input
        type='text'
        value={result}
        readOnly
        style={{ width: '100%', padding: '20px', fontSize: '24px' }}
      />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)' }}>
        <button onClick={() => handleNumberClick(7)}>7</button>
        <button onClick={() => handleNumberClick(8)}>8</button>
        <button onClick={() => handleNumberClick(9)}>9</button>
        <button onClick={handleClearClick}>C</button>
        <button onClick={() => handleNumberClick(4)}>4</button>
        <button onClick={() => handleNumberClick(5)}>5</button>
        <button onClick={() => handleNumberClick(6)}>6</button>
        <button onClick={() => handleNumberClick(1)}>1</button>
        <button onClick={() => handleNumberClick(2)}>2</button>
        <button onClick={() => handleNumberClick(3)}>3</button>
        <button onClick={() => handleNumberClick(0)}>0</button>
      </div>
    </div>
  );
};

export default Calculator;
