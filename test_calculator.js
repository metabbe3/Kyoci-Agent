// Assuming calculator functions are available in a module or globally accessible.
// For this test suite, we assume script.js exports the functions: add, subtract, multiply, divide.

const calculator = require('./script.js'); // Adjust path if necessary

function runTests() {
    console.log("--- Starting Calculator Test Suite ---");

    // Test Case 1: Addition
    let resultAdd = calculator.add(5, 3);
    console.log(`Test Add (5 + 3): Expected 8, Got ${resultAdd}. Status: ${resultAdd === 8 ? 'PASS' : 'FAIL'}`);

    // Test Case 2: Subtraction
    let resultSub = calculator.subtract(10, 4);
    console.log(`Test Subtract (10 - 4): Expected 6, Got ${resultSub}. Status: ${resultSub === 6 ? 'PASS' : 'FAIL'}`);

    // Test Case 3: Multiplication
    let resultMul = calculator.multiply(4, 5);
    console.log(`Test Multiply (4 * 5): Expected 20, Got ${resultMul}. Status: ${resultMul === 20 ? 'PASS' : 'FAIL'}`);

    // Test Case 4: Division
    let resultDiv = calculator.divide(10, 2);
    console.log(`Test Divide (10 / 2): Expected 5, Got ${resultDiv}. Status: ${resultDiv === 5 ? 'PASS' : 'FAIL'}`);

    // Edge Case: Division by Zero
    let resultDivZero = calculator.divide(10, 0);
    console.log(`Test Divide (10 / 0): Expected Infinity, Got ${resultDivZero}. Status: ${typeof resultDivZero === 'number' && !isFinite(resultDivZero) ? 'PASS' : 'FAIL'}`);
}

try {
    runTests();
} catch (e) {
    console.error("FATAL ERROR during testing:", e);
}