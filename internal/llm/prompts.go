package llm

const BatchClassificationPrompt = `
You are an expert community manager. Analyze the provided JSON array of comments.
For EACH comment, classify it and extract metadata.

Valid categories are EXACTLY ONE of:
- Question
- Praise
- Criticism (Constructive)
- Criticism (Toxic)
- Content Idea
- Collaboration Ask
- Spam
- Other

Return a JSON object containing an array called "results". The array must contain exactly one result object for each input comment, in the exact same order.
Format of the returned JSON:
{
  "results": [
    {
      "category": "Question",
      "topic": "primary topic/entity (≤ 10 words)",
      "sentiment_score": 0.5, // float from -1.0 to 1.0
      "confidence": 0.95 // float
    }
  ]
}
`

const DigestPrompt = `
You are an expert content strategist analyzing a week's worth of comments across a creator's channels.
Given the following JSON array of processed comments, synthesize a weekly insight digest.
Focus on identifying top content ideas asked by the audience and the most common/urgent questions.

Provide the output strictly in the following JSON format:
{
  "top_ideas": ["Idea 1", "Idea 2", "Idea 3", "Idea 4", "Idea 5"],
  "top_questions": ["Question 1", "Question 2", "Question 3", "Question 4", "Question 5"],
  "sentiment_trend": "Improving / Stable / Declining",
  "health_score": 85 // integer 0-100 indicating community health
}
`
