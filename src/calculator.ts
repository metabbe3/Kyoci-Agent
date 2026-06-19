interface Calculator {
    add(a: number, b: number): number;
    subtract(a: number, b: number): number;
    multiply(a: number, b: number): number;
    divide(a: number, b: number): number | null;
    calculatePercentage(value: number, percent: number): number;
    clear(): void;
    setMemory(value: number): void;
    recallMemory(): number | null;
}

export class CalculatorImpl implements Calculator {
    private memoryValue: number = 0;

    add(a: number, b: number): number {
        return a + b;
    }

    subtract(a: number, b: number): number {
        return a - b;
    }

    multiply(a: number, b: number): number {
        return a * b;
    }

    divide(a: number, b: number): number | null {
        if (b === 0) {
            console.error("Division by zero.");
            return null; // Handle division by zero gracefully
        }
        return a / b;
    }

    calculatePercentage(value: number, percent: number): number {
        return value * (percent / 100);
    }

    clear(): void {
        // Implementation depends on context, but usually clears current input/result.
    }

    setMemory(value: number): void {
        this.memoryValue = value;
    }

    recallMemory(): number | null {
        return this.memoryValue;
    }
}