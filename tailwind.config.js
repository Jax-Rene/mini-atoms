/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/*.tmpl"],
  theme: {
    extend: {
      boxShadow: {
        soft: "0 10px 30px rgba(15, 23, 42, 0.08)"
      }
    }
  },
  plugins: []
};

