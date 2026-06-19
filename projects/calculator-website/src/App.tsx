<div className="calculator">
  <input type="text" id="display" disabled />
  <div className="buttons">
    {['7', '8', '9', '/', '4', '5', '6', '*', '1', '2', '3', '-', '0', '.', '=', '+'].map((btn, index) => (
      <button key={index} onClick={() => handleButtonClick(btn)}>{btn}</button>
    ))}
  </div>
</div>