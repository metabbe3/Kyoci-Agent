import React from 'react';
import './index.css';

const Calculator: React.FC = () => {
  const [display, setDisplay] = React.useState('0');

  const handleButtonClick = (value: string) => {
    if (value === 'C') {
      setDisplay('0');
    } else if (value === '=') {
      try {
        setDisplay(eval(display).toString());
      } catch (error) {
        setDisplay('Error');
      }
    } else {
      setDisplay(display === '0' ? value : display + value);
    }
  };

  return (
    <div className='calculator'>
      <div className='display'>{display}</div>
      <div className='buttons'>
        {['7', '8', '9', '/', '4', '5', '6', '*', '1', '2', '3', '-', 'C', '0', '=', '+'].map((value, index) => (
          <button key={index} onClick={() => handleButtonClick(value)}>{value}</button>
        ))}
      </div>
    </div>
  );
};

export default Calculator;
