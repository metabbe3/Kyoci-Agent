import express from 'express';

const app = express();
const port = 3000;

// Middleware to parse JSON bodies
app.use(express.json());

// Simple root route for testing the server setup
app.get('/', (req, res) => {
  res.send('Calculator Backend Running!');
});

// TODO: Implement calculator logic routes here (e.g., POST /calculate)

app.listen(port, () => {
  console.log(`Calculator server listening at http://localhost:${port}`);
});