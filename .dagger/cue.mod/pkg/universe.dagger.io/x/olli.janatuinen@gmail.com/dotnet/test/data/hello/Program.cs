using System;

public class Program
{
	public static void Main()
	{
		var name = Environment.GetEnvironmentVariable("NAME");
		if (String.IsNullOrEmpty(name)) {
			name = "John Doe";
		}
		Console.Write(Greeting.GetMessage(name));
	}
}
